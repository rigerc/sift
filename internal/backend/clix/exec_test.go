package clix

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecRejectsNonWhitelistedFlags(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		args    []string
		want    Code
	}{
		{name: "flag not whitelisted", allowed: []string{"--json"}, args: []string{"--dangerous"}, want: CodeInvalidArgs},
		{name: "value starting with dash", allowed: []string{"--json"}, args: []string{"--json", "-rm-rf"}, want: CodeInvalidArgs},
		{name: "empty argument", allowed: []string{"--json"}, args: []string{""}, want: CodeInvalidArgs},
		{name: "control character", allowed: []string{"--json"}, args: []string{"a\x00b"}, want: CodeInvalidArgs},
		{name: "too many arguments", allowed: []string{"--json"}, args: make([]string, maxArgCount+1), want: CodeInvalidArgs},
		{name: "overlong argument", allowed: []string{"--json"}, args: []string{makeString(maxArgLen + 1)}, want: CodeInvalidArgs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Exec{}
			_, _, err := e.Run(context.Background(), "askill", tt.allowed, tt.args)
			if CodeOf(err) != tt.want {
				t.Fatalf("want code %s, got %v", tt.want, err)
			}
		})
	}
}

func makeString(n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = 'a'
	}
	return string(out)
}

func TestExecAcceptsWhitelistedFlagsAndValues(t *testing.T) {
	e := &Exec{}
	stdout, _, err := e.Run(context.Background(), "echo", []string{"-n", "--json"}, []string{"-n", "hello", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "hello --json" {
		t.Fatalf("unexpected stdout %q", stdout)
	}
}

func TestExecMapsExitCodes(t *testing.T) {
	tests := []struct {
		script string
		want   Code
	}{
		{script: "exit 2", want: CodeInvalidArgs},
		{script: "exit 3", want: CodeNotFound},
		{script: "exit 4", want: CodeAuthRequired},
		{script: "exit 5", want: CodeNetwork},
		{script: "exit 9", want: CodeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			e := &Exec{}
			_, _, err := e.Run(context.Background(), "sh", []string{"-c"}, []string{"-c", tt.script})
			if CodeOf(err) != tt.want {
				t.Fatalf("want code %s, got %v", tt.want, err)
			}
		})
	}
}

func TestExecTimesOut(t *testing.T) {
	e := &Exec{Span: 50 * time.Millisecond}
	_, _, err := e.Run(context.Background(), "sleep", []string{}, []string{"5"})
	if CodeOf(err) != CodeNetwork {
		t.Fatalf("want network code for timeout, got %v", err)
	}
}

func TestExecRedactsSecrets(t *testing.T) {
	e := &Exec{Secrets: []string{"ask_super_secret"}}
	_, _, err := e.Run(context.Background(), "sh", []string{"-c"}, []string{"-c", "echo token=ask_super_secret >&2; exit 1"})
	var cliErr *Error
	if !errors.As(err, &cliErr) {
		t.Fatalf("want *Error, got %v", err)
	}
	if got := cliErr.Message + cliErr.Raw; strings.Contains(got, "ask_super_secret") {
		t.Fatalf("secret leaked in %q", got)
	}
	if !strings.Contains(cliErr.Raw, "***") {
		t.Fatalf("expected redaction marker in %q", cliErr.Raw)
	}
}
