package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
