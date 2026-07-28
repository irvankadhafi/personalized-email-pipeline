package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
)

func generateCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "output CSV path")
	count := flags.Int64("count", 1_000_000, "recipient count")
	seed := flags.Uint64("seed", 1, "fixture seed")
	algorithm := flags.String("algorithm", recipientcsv.FixtureAlgorithmV1, "fixture algorithm")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *output == "" || *count < 0 || *algorithm != recipientcsv.FixtureAlgorithmV1 {
		return staticError(stderr, "invalid_configuration")
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return staticError(stderr, "output_unavailable")
	}
	started := time.Now()
	summary, generateErr := recipientcsv.Generate(file, recipientcsv.FixtureOptions{Algorithm: *algorithm, Seed: *seed, Count: *count})
	closeErr := file.Close()
	if generateErr != nil || closeErr != nil {
		_ = os.Remove(*output)
		return staticError(stderr, "fixture_generation_failed")
	}
	document := struct {
		Outcome                  string                      `json:"outcome"`
		GenerationElapsedSeconds float64                     `json:"generation_elapsed_seconds"`
		Expected                 recipientcsv.FixtureSummary `json:"expected"`
	}{Outcome: "success", GenerationElapsedSeconds: time.Since(started).Seconds(), Expected: summary}
	if json.NewEncoder(stdout).Encode(document) != nil {
		return staticError(stderr, "report_write_failed")
	}
	return 0
}
