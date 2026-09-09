// Package clix is the process-execution layer shared by CLI backends. It
// centralizes argument whitelisting and sanitization, per-call timeouts,
// secret redaction, pinned-CLI-version verification, and a normalized error
// vocabulary so adapters never leak backend-specific failure strings upward.
package clix

import "fmt"

// Code is the shared normalized error vocabulary. Callers branch on Code,
// never on backend-specific strings or exit codes.
type Code string

const (
	CodeNotFound     Code = "not_found"
	CodeAuthRequired Code = "auth_required"
	CodeNetwork      Code = "network"
	CodeInvalidArgs  Code = "invalid_arguments"
	CodeAmbiguous    Code = "ambiguous_selection"
	CodeUnknown      Code = "unknown"
	CodeVersionDrift Code = "version_drift"
)

// Error is the normalized failure shape returned by every CLI call.
// Raw carries the unredacted-safe backend output for triage.
type Error struct {
	Code     Code
	Message  string
	ExitCode int
	Raw      string
}

func (e *Error) Error() string {
	if e.Raw != "" {
		return fmt.Sprintf("%s: %s (exit=%d, raw=%q)", e.Code, e.Message, e.ExitCode, e.Raw)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s: %s (exit=%d)", e.Code, e.Message, e.ExitCode)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// CodeOf extracts the normalized Code from an error, defaulting to unknown.
func CodeOf(err error) Code {
	var e *Error
	if ok := asError(err, &e); ok {
		return e.Code
	}
	return CodeUnknown
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

func newError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
