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

func TestRuntimeDependencyGraphHasNoNetworkPackages(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/email-pipeline")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range strings.Fields(string(output)) {
		if imported == "net" || strings.HasPrefix(imported, "net/") {
			t.Fatalf("dial-capable dependency: %s", imported)
		}
	}
}
