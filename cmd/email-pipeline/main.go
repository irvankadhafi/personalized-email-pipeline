package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
)

const usage = `email-pipeline performs offline personalized-email dry runs.

Usage:
  email-pipeline generate --output PATH [--count N] [--seed N] [--algorithm v1]
  email-pipeline run --input PATH [--workers N] [--settlement 5s]
  email-pipeline help

Exit codes: 0 success, 1 failure, 2 partial success, 130 interrupted.
`

var panicHook = func() {}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			_, _ = io.WriteString(stderr, "error: internal_failure\n")
			code = 1
		}
	}()
	panicHook()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	switch args[0] {
	case "generate":
		return generateCommand(args[1:], stdout, stderr)
	case "run":
		return runCommand(args[1:], stdout, stderr)
	default:
		_, _ = io.WriteString(stderr, "error: invalid_command\n")
		return 1
	}
}

func generateCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "output CSV path")
	count := flags.Int64("count", 1_000_000, "recipient count")
	seed := flags.Uint64("seed", 1, "fixture seed")
	algorithm := flags.String("algorithm", recipientcsv.FixtureAlgorithmV1, "fixture algorithm")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *output == "" || *count < 0 || *algorithm != recipientcsv.FixtureAlgorithmV1 {
		_, _ = io.WriteString(stderr, "error: invalid_configuration\n")
		return 1
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_, _ = io.WriteString(stderr, "error: output_unavailable\n")
		return 1
	}
	started := time.Now()
	summary, generateErr := recipientcsv.Generate(file, recipientcsv.FixtureOptions{Algorithm: *algorithm, Seed: *seed, Count: *count})
	closeErr := file.Close()
	if generateErr != nil || closeErr != nil {
		_ = os.Remove(*output)
		_, _ = io.WriteString(stderr, "error: fixture_generation_failed\n")
		return 1
	}
	document := struct {
		Outcome                  string                      `json:"outcome"`
		GenerationElapsedSeconds float64                     `json:"generation_elapsed_seconds"`
		Expected                 recipientcsv.FixtureSummary `json:"expected"`
	}{Outcome: "success", GenerationElapsedSeconds: time.Since(started).Seconds(), Expected: summary}
	if json.NewEncoder(stdout).Encode(document) != nil {
		_, _ = io.WriteString(stderr, "error: report_write_failed\n")
		return 1
	}
	return 0
}

func runCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "input CSV path")
	workers := flags.Int("workers", runtime.GOMAXPROCS(0), "render workers")
	settlement := flags.Duration("settlement", campaign.DefaultSettlement, "cancellation settlement deadline")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *input == "" || *workers <= 0 || *settlement <= 0 {
		_, _ = io.WriteString(stderr, "error: invalid_configuration\n")
		return 1
	}
	file, err := os.Open(*input)
	if err != nil {
		_, _ = io.WriteString(stderr, "error: input_unavailable\n")
		return 1
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_, _ = io.WriteString(stderr, "error: input_not_regular\n")
		return 1
	}
	reader, err := recipientcsv.NewReader(file)
	if err != nil {
		data, reportErr := campaign.MarshalReport(campaign.RunReport{
			Outcome: campaign.Failure, AccountingScope: "prefix_only", Fatal: true,
		})
		if reportErr != nil {
			_, _ = io.WriteString(stderr, "error: fatal_input\n")
			return 1
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report := campaign.Run(ctx, func() (campaign.SourceRecord, bool, error) {
		record, ok, readErr := reader.Next()
		return campaign.SourceRecord{Ordinal: record.Ordinal, Email: record.Email, Name: record.Name, ParserReason: record.Reason}, ok, readErr
	}, campaign.RunConfig{Workers: *workers, Settlement: *settlement})
	data, err := campaign.MarshalReport(report)
	if err != nil {
		_, _ = io.WriteString(stderr, "error: untrustworthy_accounting\n")
		return 1
	}
	if _, err := fmt.Fprintln(stdout, string(data)); err != nil {
		_, _ = io.WriteString(stderr, "error: report_write_failed\n")
		return 1
	}
	switch report.Outcome {
	case campaign.Success:
		return 0
	case campaign.PartialSuccess:
		return 2
	case campaign.Interrupted:
		return 130
	case campaign.Failure:
		return 1
	default:
		return 1
	}
}
