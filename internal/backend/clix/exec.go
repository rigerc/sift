package clix

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// Runner executes one command invocation. allowed lists the literal flags the
// caller is permitted to pass; every arg starting with "-" must appear in it.
type Runner interface {
	Run(ctx context.Context, binary string, allowed []string, args []string) (stdout, stderr []byte, err error)
}

const (
	maxArgLen   = 256
	maxArgCount = 32
	defaultSpan = 30 * time.Second
)

// Exec spawns real processes. Secrets are redacted from any error text before
// it leaves the layer, and every run is bounded by a timeout.
type Exec struct {
	// Secrets are redacted from error messages and captured output.
	Secrets []string
	// Span bounds a single command; zero selects the 30s default.
	Span time.Duration
}

func (e *Exec) Run(ctx context.Context, binary string, allowed []string, args []string) ([]byte, []byte, error) {
	if err := validateBinary(binary); err != nil {
		return nil, nil, err
	}
	if err := validateArgs(allowed, args); err != nil {
		return nil, nil, err
	}
	span := e.Span
	if span <= 0 {
		span = defaultSpan
	}
	runCtx, cancel := context.WithTimeout(ctx, span)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, e.redact(stderr.Bytes()), e.failure(runCtx, err, e.redact(stderr.Bytes()))
	}
	return e.redact(stdout.Bytes()), e.redact(stderr.Bytes()), nil
}

func (e *Exec) failure(ctx context.Context, err error, stderr []byte) error {
	message := strings.TrimSpace(string(stderr))
	raw := message
	if message == "" {
		message = err.Error()
		raw = message
	}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return newError(CodeNetwork, "command timed out after %s: %s", e.spanString(), message)
	case errors.As(err, &exitErr):
		code := exitErr.ExitCode()
		return &Error{Code: mapExitCode(code), Message: message, ExitCode: code, Raw: raw}
	default:
		return &Error{Code: CodeUnknown, Message: message, Raw: raw}
	}
}

func (e *Exec) spanString() string {
	if e.Span <= 0 {
		return defaultSpan.String()
	}
	return e.Span.String()
}

func (e *Exec) redact(data []byte) []byte {
	out := data
	for _, secret := range e.Secrets {
		if secret == "" {
			continue
		}
		out = bytes.ReplaceAll(out, []byte(secret), []byte("***"))
	}
	return out
}

// mapExitCode provisionally maps askill's documented exit codes; backends
// without a documented contract fall back to unknown.
func mapExitCode(code int) Code {
	switch code {
	case 2:
		return CodeInvalidArgs
	case 3:
		return CodeNotFound
	case 4:
		return CodeAuthRequired
	case 5:
		return CodeNetwork
	default:
		return CodeUnknown
	}
}

func validateBinary(binary string) error {
	if binary == "" {
		return newError(CodeInvalidArgs, "binary name is empty")
	}
	if strings.HasPrefix(binary, "-") {
		return newError(CodeInvalidArgs, "binary %q looks like a flag", binary)
	}
	if strings.ContainsFunc(binary, func(r rune) bool { return unicode.IsControl(r) || r == 0 }) {
		return newError(CodeInvalidArgs, "binary %q contains control characters", binary)
	}
	return nil
}

func validateArgs(allowed []string, args []string) error {
	if len(args) > maxArgCount {
		return newError(CodeInvalidArgs, "%d arguments exceed limit of %d", len(args), maxArgCount)
	}
	permitted := make(map[string]bool, len(allowed))
	for _, f := range allowed {
		permitted[f] = true
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if !permitted[arg] {
				return newError(CodeInvalidArgs, "flag %q is not whitelisted", arg)
			}
			continue
		}
		if arg == "" {
			return newError(CodeInvalidArgs, "empty argument")
		}
		if len(arg) > maxArgLen {
			return newError(CodeInvalidArgs, "argument exceeds %d characters", maxArgLen)
		}
		if strings.ContainsFunc(arg, func(r rune) bool { return unicode.IsControl(r) || r == 0 }) {
			return newError(CodeInvalidArgs, "argument %q contains control characters", arg)
		}
	}
	return nil
}
