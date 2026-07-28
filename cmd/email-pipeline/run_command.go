package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/distributed"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/testinbox"
	"github.com/redis/go-redis/v9"
)

func runCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	options := runOptions{}
	var backend, sink string
	flags.StringVar(&options.input, "input", "", "input CSV path")
	flags.StringVar(&backend, "backend", string(campaign.BackendLocal), "execution backend")
	flags.StringVar(&sink, "sink", string(campaign.SinkDryRun), "message sink")
	flags.Int64Var(&options.count, "count", 0, "generated recipient count")
	flags.Uint64Var(&options.seed, "seed", 1, "fixture seed")
	flags.StringVar(&options.algorithm, "algorithm", recipientcsv.FixtureAlgorithmV1, "fixture algorithm")
	flags.IntVar(&options.workers, "workers", runtime.GOMAXPROCS(0), "render workers")
	flags.DurationVar(&options.settlement, "settlement", campaign.DefaultSettlement, "cancellation settlement deadline")
	flags.StringVar(&options.confirmation, "confirm-test-inbox", "", "test-inbox confirmation")
	flags.Int64Var(&options.taskSize, "task-size", 100, "generated records per task")
	flags.IntVar(&options.maxRetry, "max-retry", 3, "maximum task retries")
	flags.DurationVar(&options.completionDeadline, "completion-deadline", 30*time.Second, "distributed completion deadline")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return staticError(stderr, "invalid_configuration")
	}
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "backend" || value.Name == "sink" {
			options.exposeMode = true
		}
	})
	options.backend, options.sink = campaign.Backend(backend), campaign.Sink(sink)
	if options.validate() != nil {
		return staticError(stderr, "guard_refused")
	}
	if options.backend == campaign.BackendLocal {
		return runLocal(options, stdout, stderr)
	}
	return runDistributed(options, stdout, stderr)
}

func runLocal(options runOptions, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if options.sink == campaign.SinkDryRun {
		return runLocalFile(ctx, options, stdout, stderr)
	}
	smtp, err := smtpSinkFromEnvironment()
	if err != nil {
		return staticError(stderr, "guard_refused")
	}
	source, err := recipientcsv.NewGeneratedSource(recipientcsv.FixtureOptions{Algorithm: options.algorithm, Seed: options.seed, Count: options.count})
	if err != nil {
		return staticError(stderr, "invalid_configuration")
	}
	var confirmed, rejected, transport, indeterminate atomic.Int64
	report := campaign.Run(ctx, func() (campaign.SourceRecord, bool, error) {
		record, ok := source.Next()
		return campaign.SourceRecord{Ordinal: record.Ordinal, Email: record.Email, Name: record.Name}, ok, nil
	}, campaign.RunConfig{Workers: options.workers, Settlement: options.settlement, Sink: func(ctx context.Context, body []byte) error {
		result := smtp.Deliver(ctx, body)
		switch result.Status {
		case testinbox.DeliveryConfirmed:
			confirmed.Add(1)
		case testinbox.DeliveryIndeterminate:
			indeterminate.Add(1)
		case testinbox.DeliveryTransport:
			transport.Add(1)
		default:
			rejected.Add(1)
		}
		return result.Err
	}})
	report.Mode = &campaign.ModeEvidence{Backend: options.backend, Sink: options.sink}
	report.TestDelivery = &campaign.TestDeliveryEvidence{
		Confirmed: confirmed.Load(), Rejected: rejected.Load(), Transport: transport.Load(), Indeterminate: indeterminate.Load(),
	}
	return writeRunReport(report, stdout, stderr)
}

func runLocalFile(ctx context.Context, options runOptions, stdout, stderr io.Writer) int {
	file, err := os.Open(options.input)
	if err != nil {
		return staticError(stderr, "input_unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return staticError(stderr, "input_not_regular")
	}
	reader, err := recipientcsv.NewReader(file)
	if err != nil {
		return writeRunReport(campaign.RunReport{Outcome: campaign.Failure, AccountingScope: "prefix_only", Fatal: true}, stdout, stderr)
	}
	report := campaign.Run(ctx, func() (campaign.SourceRecord, bool, error) {
		record, ok, readErr := reader.Next()
		return campaign.SourceRecord{Ordinal: record.Ordinal, Email: record.Email, Name: record.Name, ParserReason: record.Reason}, ok, readErr
	}, campaign.RunConfig{Workers: options.workers, Settlement: options.settlement})
	if options.exposeMode {
		report.Mode = &campaign.ModeEvidence{Backend: options.backend, Sink: options.sink}
	}
	return writeRunReport(report, stdout, stderr)
}

func runDistributed(options runOptions, stdout, stderr io.Writer) int {
	var smtp *testinbox.Sink
	var err error
	if options.sink == campaign.SinkTestInbox {
		smtp, err = smtpSinkFromEnvironment()
		if err != nil {
			return staticError(stderr, "guard_refused")
		}
	}
	redisCfg, err := redisConfigFromEnvironment()
	if err != nil {
		return staticError(stderr, "distributed_unavailable")
	}
	redisClient := redis.NewClient(redisCfg.redisOptions())
	defer redisClient.Close()
	ledger, err := distributed.NewLedger(redisClient)
	if err != nil {
		return staticError(stderr, "distributed_unavailable")
	}
	asynqClient := asynq.NewClient(redisCfg.asynqOptions())
	defer asynqClient.Close()
	inspector := asynq.NewInspector(redisCfg.asynqOptions())
	defer inspector.Close()
	now := time.Now().UTC()
	policy := distributed.PolicyRetryable
	deliveryLimit := int64(0)
	taskSink := distributed.TaskSinkDryRun
	if options.sink == campaign.SinkTestInbox {
		policy, deliveryLimit, taskSink = distributed.PolicyOneShotDelivery, distributed.MaxDeliveryLimit, distributed.TaskSinkTestInbox
	}
	campaignID := fmt.Sprintf("campaign-%d", now.UnixNano())
	producer, err := distributed.NewProducer(distributed.ProducerConfig{
		Ledger: ledger, Enqueuer: distributed.AsynqEnqueuer{Client: asynqClient}, Inspector: inspector,
		Campaign: distributed.Campaign{
			ID: campaignID, Algorithm: options.algorithm, Seed: options.seed, Total: options.count,
			Created: now, Deadline: now.Add(options.completionDeadline), Version: distributed.LedgerVersion,
			Policy: policy, DeliveryLimit: deliveryLimit,
		},
		Sink: taskSink, TaskSize: options.taskSize, MaxRetry: options.maxRetry,
		Retention: time.Hour, EnqueueTimeout: 5 * time.Second, PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		return staticError(stderr, "invalid_configuration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, enqueueErr := producer.Enqueue(ctx)
	if enqueueErr == nil {
		result, enqueueErr = producer.Wait(ctx, result)
	}
	_ = smtp
	report := distributedReport(options, result, enqueueErr)
	return writeRunReport(report, stdout, stderr)
}

func distributedReport(options runOptions, result distributed.ProducerResult, runErr error) campaign.RunReport {
	snapshot := result.Snapshot
	classified := snapshot.Completed + snapshot.Failed + snapshot.Unprocessed
	if result.Unknown == 0 {
		classified = snapshot.Total
	}
	counts := campaign.Counts{
		Examined: classified, Eligible: classified, Completed: snapshot.Completed,
		Failed: snapshot.Failed, Unprocessed: snapshot.Unprocessed,
	}
	fatal := runErr != nil && !errors.Is(runErr, context.Canceled)
	scope := "full"
	if result.Unknown > 0 {
		scope = "unknown"
	} else if fatal {
		scope = "prefix_only"
	}
	report := campaign.RunReport{
		AccountingScope: scope, Counts: counts, Started: snapshot.Completed + snapshot.Failed,
		Fatal: fatal, Cancelled: runErr == context.Canceled,
		Mode: &campaign.ModeEvidence{Backend: options.backend, Sink: options.sink},
		Distributed: &campaign.DistributedEvidence{
			KnownEnqueued: result.KnownEnqueued, KnownTerminal: snapshot.KnownTerminal(), Unknown: result.Unknown,
		},
	}
	if options.sink == campaign.SinkTestInbox {
		report.TestDelivery = &campaign.TestDeliveryEvidence{
			Confirmed:     snapshot.Completed,
			Rejected:      snapshot.DeliveryRejected,
			Transport:     snapshot.DeliveryTransport,
			Indeterminate: snapshot.DeliveryUnknown,
		}
	}
	report.Outcome = campaign.DeriveOutcome(report.Counts, report.Cancelled, report.Fatal)
	return report
}

func writeRunReport(report campaign.RunReport, stdout, stderr io.Writer) int {
	data, err := campaign.MarshalReport(report)
	if err != nil {
		return staticError(stderr, "untrustworthy_accounting")
	}
	if _, err := fmt.Fprintln(stdout, string(data)); err != nil {
		return staticError(stderr, "report_write_failed")
	}
	switch report.Outcome {
	case campaign.Success:
		return 0
	case campaign.PartialSuccess:
		return 2
	case campaign.Interrupted:
		return 130
	default:
		return 1
	}
}

func staticError(stderr io.Writer, reason string) int {
	_, _ = fmt.Fprintf(stderr, "error: %s\n", reason)
	return 1
}
