// Package prompt provides standalone huh/v2 interactive prompts for the
// headless-first CLI. Every prompt runs as a blocking huh form directly on
// the terminal; nothing here starts a BubbleTea program or imports Cobra.
// The full-screen TUI under internal/ui remains available via `skillscan tui`.
package prompt

import (
	"errors"
	"fmt"
	"go-s/internal/ui/theme"
	"os"

	huh "charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// ErrNonTTY is returned when an interactive prompt is requested without a terminal.
var ErrNonTTY = errors.New("prompt: interactive terminal required")

// IsTTY reports whether the process has an interactive terminal attached.
func IsTTY() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// RequireTTY explains the headless alternatives when a prompt needs a terminal.
func RequireTTY(what string) error {
	return fmt.Errorf("%w for %s; use --json or --dry-run for headless output", ErrNonTTY, what)
}

func isAbort(err error) bool { return errors.Is(err, huh.ErrUserAborted) }

// ShowWelcome presents the first-run note. Abort counts as acknowledged.
func ShowWelcome(themeName string) error {
	if !IsTTY() {
		return RequireTTY("welcome")
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().
			Title("Welcome to skillscan").
			Description("Scan a workspace for technologies and suggest installable agent skills.\n\nRun `skillscan scan` for the guided flow, `scan --json` for machine output, or `skillscan agent` for an AI-agent brief.").
			Next(true).
			NextLabel("Get started"),
	)).WithTheme(theme.HuhTheme(themeName))
	if err := form.Run(); err != nil {
		if isAbort(err) {
			return nil
		}
		return err
	}
	return nil
}
