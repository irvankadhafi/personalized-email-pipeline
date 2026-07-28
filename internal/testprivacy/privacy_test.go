package testprivacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIArtifactsExcludeCanaries(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	dir := t.TempDir()
	input := filepath.Join(dir, "recipients.csv")
	stdout := filepath.Join(dir, "privacy-canary-stdout")
	stderr := filepath.Join(dir, "privacy-canary-stderr")
	canaries := []string{"SECRET_CREDENTIAL_9f3a", "Identifiable Name", "private.person@example.test"}
	content := "email,name\n" + canaries[2] + "," + canaries[1] + "\n"
	if err := os.WriteFile(input, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	outFile, err := os.OpenFile(stdout, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	errFile, err := os.OpenFile(stderr, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		outFile.Close()
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "./cmd/email-pipeline", "run", "--input", input, "--workers", "1")
	command.Dir = root
	command.Stdout = outFile
	command.Stderr = errFile
	command.Env = append(os.Environ(), "PRIVACY_CANARY="+canaries[0])
	runErr := command.Run()
	closeOutErr := outFile.Close()
	closeErrErr := errFile.Close()
	if runErr != nil || closeOutErr != nil || closeErrErr != nil {
		t.Fatal("privacy subprocess failed")
	}
	for _, path := range []string{stdout, stderr} {
		artifact, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, canary := range canaries {
			if strings.Contains(string(artifact), canary) {
				t.Fatal("privacy canary leaked")
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultPathIgnoresOptionalNetworkConfiguration(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	dir := t.TempDir()
	input := filepath.Join(dir, "recipients.csv")
	if err := os.WriteFile(input, []byte("email,name\nrecipient@example.test,Recipient\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "./cmd/email-pipeline", "run", "--input", input, "--workers", "1")
	command.Dir = root
	command.Env = append(os.Environ(),
		"EMAIL_PIPELINE_REDIS_ADDR=127.0.0.1:1",
		"EMAIL_PIPELINE_REDIS_DB=0",
		"EMAIL_PIPELINE_SMTP_HOST=127.0.0.1",
		"EMAIL_PIPELINE_SMTP_PORT=1",
		"EMAIL_PIPELINE_SMTP_USERNAME=SECRET_CREDENTIAL_USERNAME",
		"EMAIL_PIPELINE_SMTP_PASSWORD=SECRET_CREDENTIAL_PASSWORD",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("default subprocess failed: %s", output)
	}
	if !strings.Contains(string(output), `"outcome":"success"`) {
		t.Fatalf("default path did not complete: %s", output)
	}
	for _, canary := range []string{"SECRET_CREDENTIAL_USERNAME", "SECRET_CREDENTIAL_PASSWORD", "127.0.0.1:1"} {
		if strings.Contains(string(output), canary) {
			t.Fatalf("optional configuration leaked: %q", canary)
		}
	}
}

func TestOptionalFailureOutputExcludesConfigurationCanaries(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	command := exec.Command("go", "run", "./cmd/email-pipeline", "run", "--backend", "asynq", "--sink", "dry-run", "--count", "1", "--completion-deadline", "10ms")
	command.Dir = root
	canaries := []string{"SECRET_REDIS_USERNAME", "SECRET_REDIS_PASSWORD", "127.0.0.1:1"}
	command.Env = append(os.Environ(),
		"EMAIL_PIPELINE_REDIS_ADDR="+canaries[2],
		"EMAIL_PIPELINE_REDIS_USERNAME="+canaries[0],
		"EMAIL_PIPELINE_REDIS_PASSWORD="+canaries[1],
		"EMAIL_PIPELINE_REDIS_DB=0",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("optional subprocess unexpectedly succeeded")
	}
	if !strings.Contains(string(output), `"accounting_scope":"unknown"`) {
		t.Fatalf("missing unknown-state report: %s", output)
	}
	for _, canary := range canaries {
		if strings.Contains(string(output), canary) {
			t.Fatalf("optional failure leaked configuration: %q", canary)
		}
	}
}
