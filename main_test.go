package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLIErrorBranding(t *testing.T) {
	if os.Getenv("SIFT_TEST_CLI_ERROR") == "1" {
		os.Args = []string{"sift", "unknown-command"}
		os.Exit(run())
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestCLIErrorBranding$")
	child.Env = append(os.Environ(), "SIFT_TEST_CLI_ERROR=1")
	var stdout, stderr bytes.Buffer
	child.Stdout, child.Stderr = &stdout, &stderr
	err = child.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("CLI status: %v", err)
	}
	if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "sift: unknown command") || strings.Contains(stderr.String(), "skillscan") {
		t.Fatalf("CLI error output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
