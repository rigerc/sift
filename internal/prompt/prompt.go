// Package prompt provides bounded inline Huh selection. Chrome goes to stderr;
// stdout is reserved for the resulting plan, and no alternate screen is used.
package prompt

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
)

var ErrNonTTY = errors.New("prompt: interactive terminal required")

// IsTTY requires terminal input, output, and chrome. Redirected scans must
// remain headless rather than unexpectedly prompting in a shell pipeline.
func IsTTY() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

func RequireTTY(what string) error {
	return fmt.Errorf("%w for %s; use --json or --dry-run for headless output", ErrNonTTY, what)
}
