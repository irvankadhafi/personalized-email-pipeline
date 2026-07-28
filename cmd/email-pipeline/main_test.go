package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/distributed"
)

func TestGenerateThenRun(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.csv")
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"generate", "--output", fixture, "--count", "4", "--seed", "7"}, &stdout, &stderr); code != 0 {
		t.Fatalf("generate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var generated struct {
		Expected struct {
			Count int64 `json:"count"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &generated); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := execute([]string{"run", "--input", fixture, "--workers", "2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"outcome":"success"`) || strings.Contains(stdout.String(), "recipient-000001-") {
		t.Fatalf("unsafe or invalid report: %s", stdout.String())
	}
	var processed struct {
		Counts struct {
			Examined  int64 `json:"examined"`
			Completed int64 `json:"completed"`
		} `json:"counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &processed); err != nil {
		t.Fatal(err)
	}
	if processed.Counts.Examined != generated.Expected.Count || processed.Counts.Completed != generated.Expected.Count {
		t.Fatalf("generated=%d processed=%#v", generated.Expected.Count, processed.Counts)
	}
}

func TestRunRejectsNonRegularInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"run", "--input", t.TempDir()}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(stderr.String(), t.TempDir()) {
		t.Fatal("stderr leaked path")
	}
}

func TestExecuteRecoversPanicWithoutValue(t *testing.T) {
	old := panicHook
	panicHook = func() { panic("SECRET_CREDENTIAL") }
	defer func() { panicHook = old }()
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"help"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(stderr.String(), "SECRET_CREDENTIAL") || strings.Contains(stderr.String(), "goroutine") {
		t.Fatalf("panic leaked: %s", stderr.String())
	}
}

func TestMainHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"help"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "generate") || !strings.Contains(stdout.String(), "run") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestExplicitDefaultSelectorsAppearInReport(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "fixture.csv")
	if err := os.WriteFile(fixture, []byte("email,name\nrecipient@example.test,Recipient\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := execute([]string{"run", "--input", fixture, "--backend", "local", "--sink", "dry-run"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"mode":{"backend":"local","sink":"dry-run"}`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestTestInboxGuardRefusesBeforeConfiguration(t *testing.T) {
	t.Setenv("EMAIL_PIPELINE_SMTP_PASSWORD", "SECRET_CREDENTIAL")
	tests := [][]string{
		{"run", "--backend", "local", "--sink", "test-inbox", "--count", "1"},
		{"run", "--backend", "local", "--sink", "test-inbox", "--count", "1", "--confirm-test-inbox", testInboxConfirmation, "--format", "markdown"},
		{"run", "--backend", "asynq", "--sink", "test-inbox", "--count", "11", "--confirm-test-inbox", testInboxConfirmation},
		{"run", "--backend", "asynq", "--sink", "test-inbox", "--input", "private.csv", "--confirm-test-inbox", testInboxConfirmation},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if code := execute(args, &stdout, &stderr); code != 1 || stderr.String() != "error: guard_refused\n" {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "SECRET_CREDENTIAL") {
			t.Fatal("configuration leaked")
		}
	}
}

func TestAsynqFailureDoesNotFallbackLocally(t *testing.T) {
	t.Setenv("EMAIL_PIPELINE_REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("EMAIL_PIPELINE_REDIS_DB", "0")
	var stdout, stderr bytes.Buffer
	code := execute([]string{"run", "--backend", "asynq", "--sink", "dry-run", "--count", "1", "--completion-deadline", "10ms"}, &stdout, &stderr)
	if code != 1 || strings.Contains(stdout.String(), `"completed":1`) ||
		!strings.Contains(stdout.String(), `"accounting_scope":"unknown"`) ||
		!strings.Contains(stdout.String(), `"unknown":1`) || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestAsynqHTMLPassesFormatValidationBeforeRedisConnection(t *testing.T) {
	// Given
	t.Setenv("EMAIL_PIPELINE_REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("EMAIL_PIPELINE_REDIS_DB", "0")
	var stdout, stderr bytes.Buffer

	// When
	code := execute([]string{"run", "--backend", "asynq", "--sink", "dry-run", "--format", "html", "--count", "1", "--completion-deadline", "10ms"}, &stdout, &stderr)

	// Then
	if code != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"accounting_scope":"unknown"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestDistributedUnknownReportReconcilesOnlyTrustworthyWork(t *testing.T) {
	report := distributedReport(runOptions{backend: campaign.BackendAsynq, sink: campaign.SinkDryRun}, distributed.ProducerResult{
		Snapshot:      distributed.Snapshot{Total: 4, Completed: 1, Unprocessed: 1},
		KnownEnqueued: 2,
		Unknown:       2,
	}, distributed.ErrUnknownState)
	var stdout, stderr bytes.Buffer

	code := writeRunReport(report, &stdout, &stderr)

	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if report.Counts != (campaign.Counts{Examined: 2, Eligible: 2, Completed: 1, Unprocessed: 1}) ||
		report.Outcome != campaign.Failure || report.AccountingScope != "unknown" || !report.Fatal {
		t.Fatalf("report=%+v", report)
	}
	if !strings.Contains(stdout.String(), `"distributed":{"known_enqueued":2,"known_terminal":1,"unknown":2}`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestDistributedDeadlineProducesFailure(t *testing.T) {
	report := distributedReport(runOptions{backend: campaign.BackendAsynq, sink: campaign.SinkDryRun}, distributed.ProducerResult{
		Snapshot:      distributed.Snapshot{Total: 1, Failed: 1},
		KnownEnqueued: 1,
	}, distributed.ErrCompletionDeadline)

	if report.Outcome != campaign.Failure || !report.Fatal || report.AccountingScope != "prefix_only" {
		t.Fatalf("report=%+v", report)
	}
}

func TestWorkerRequiresExplicitSink(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"worker"}, &stdout, &stderr); code != 1 || stderr.String() != "error: invalid_configuration\n" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestRunHeaderFailureEmitsJSONReport(t *testing.T) {
	input := filepath.Join(t.TempDir(), "wrong.csv")
	if err := os.WriteFile(input, []byte("wrong,header\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"run", "--input", input}, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(stdout.String(), `"outcome":"failure"`) || !strings.Contains(stdout.String(), `"fatal":true`) {
		t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "wrong") || strings.Contains(stderr.String(), input) {
		t.Fatalf("input leaked: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
