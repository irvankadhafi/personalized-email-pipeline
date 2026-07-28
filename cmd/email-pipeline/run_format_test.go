package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestRunOmittedFormatMatchesExplicitText(t *testing.T) {
	// Given
	fixture := filepath.Join(t.TempDir(), "fixture.csv")
	if err := os.WriteFile(fixture, []byte("email,name\nrecipient@example.test,Recipient\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (int, []byte, string) {
		var stdout, stderr bytes.Buffer
		code := execute(append([]string{"run", "--input", fixture, "--workers", "1"}, args...), &stdout, &stderr)
		return code, stdout.Bytes(), stderr.String()
	}

	// When
	omittedCode, omittedReport, omittedError := run()
	explicitCode, explicitReport, explicitError := run("--format", "text")

	// Then
	if omittedCode != 0 || explicitCode != 0 || omittedError != "" || explicitError != "" {
		t.Fatalf("omitted=(%d,%q) explicit=(%d,%q)", omittedCode, omittedError, explicitCode, explicitError)
	}
	var omitted, explicit struct {
		Outcome         campaign.Outcome  `json:"outcome"`
		AccountingScope string            `json:"accounting_scope"`
		Counts          campaign.Counts   `json:"counts"`
		Samples         []campaign.Sample `json:"samples"`
		Cancelled       bool              `json:"cancelled"`
		Fatal           bool              `json:"fatal"`
	}
	if err := json.Unmarshal(omittedReport, &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(explicitReport, &explicit); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(omitted, explicit) {
		t.Fatalf("omitted=%s explicit=%s", omittedReport, explicitReport)
	}
}

func TestRunRejectsInvalidFormatBeforeOpeningInput(t *testing.T) {
	// Given
	missing := filepath.Join(t.TempDir(), "missing.csv")
	var stdout, stderr bytes.Buffer

	// When
	code := execute([]string{"run", "--input", missing, "--format", "markdown"}, &stdout, &stderr)

	// Then
	if code != 1 || stdout.Len() != 0 || stderr.String() != "error: guard_refused\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunLocalHTMLPreservesAccountingAndExitCode(t *testing.T) {
	// Given
	fixture := filepath.Join(t.TempDir(), "fixture.csv")
	if err := os.WriteFile(fixture, []byte("email,name\na@example.test,A\nb@example.test,\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	// When
	code := execute([]string{"run", "--input", fixture, "--workers", "1", "--format", "html"}, &stdout, &stderr)

	// Then
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var report struct {
		Counts campaign.Counts `json:"counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Counts != (campaign.Counts{Examined: 2, Eligible: 2, Completed: 2}) {
		t.Fatalf("counts=%#v", report.Counts)
	}
}
