package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/distributed"
	"github.com/redis/go-redis/v9"
)

func workerCommand(args []string, _ io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("worker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var sink string
	concurrency := flags.Int("concurrency", runtime.GOMAXPROCS(0), "worker concurrency")
	shutdownTimeout := flags.Duration("shutdown-timeout", campaign.DefaultSettlement, "worker shutdown timeout")
	flags.StringVar(&sink, "sink", "", "worker sink")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *concurrency <= 0 || *shutdownTimeout <= 0 ||
		(sink != string(campaign.SinkDryRun) && sink != string(campaign.SinkTestInbox)) {
		return staticError(stderr, "invalid_configuration")
	}
	var deliver distributed.DeliveryFunc
	queue := distributed.TaskTypeDryRun
	if sink == string(campaign.SinkTestInbox) {
		smtp, err := smtpSinkFromEnvironment()
		if err != nil {
			return staticError(stderr, "guard_refused")
		}
		deliver, queue = smtp.DeliverMessage, distributed.TaskTypeTestInbox
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
	worker, err := distributed.NewWorker(distributed.WorkerConfig{Ledger: ledger, Deliver: deliver})
	if err != nil {
		return staticError(stderr, "invalid_configuration")
	}
	server := asynq.NewServer(redisCfg.asynqOptions(), asynq.Config{
		Concurrency: *concurrency, Queues: map[string]int{queue: 1}, ShutdownTimeout: *shutdownTimeout,
		LogLevel: asynq.ErrorLevel, ErrorHandler: worker,
	})
	mux := asynq.NewServeMux()
	mux.Handle(queue, worker)
	if err := server.Start(mux); err != nil {
		return staticError(stderr, "distributed_unavailable")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()
	server.Stop()
	server.Shutdown()
	return 0
}
