package main

import (
	"io"
	"os"
)

const usage = `email-pipeline performs personalized-email demonstrations.

Usage:
  email-pipeline generate --output PATH [--count N] [--seed N] [--algorithm v1]
  email-pipeline run --input PATH [--backend local] [--sink dry-run]
  email-pipeline run --backend local|asynq --sink dry-run|test-inbox --count N [options]
  email-pipeline worker --sink dry-run|test-inbox [--concurrency N] [--shutdown-timeout 5s]
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
	case "worker":
		return workerCommand(args[1:], stdout, stderr)
	default:
		_, _ = io.WriteString(stderr, "error: invalid_command\n")
		return 1
	}
}
