package cmd

import (
	"go-s/internal/app"
	"go-s/internal/install"

	"github.com/spf13/cobra"
)

// TUIRequest describes a full-screen BubbleTea session requested from the CLI.
// It is the only path that starts a tea.Program: `skillscan tui`,
// `skillscan --tui`, `skillscan scan --tui|--interactive`, or
// `skillscan install --tui`. All other flows use headless output or the
// standalone huh prompts in internal/prompt.
type TUIRequest struct {
	Screen   string // "home", "scan", or "install"
	ScanRoot string
	ScanOpts app.ScanOptions
	Plan     install.Plan
}

// tuiReq holds the pending TUI request after a successful Execute.
var tuiReq *TUIRequest

// RequestedTUI returns the pending full-screen UI request, if any.
func RequestedTUI() (*TUIRequest, bool) {
	if tuiReq == nil {
		return nil, false
	}
	return tuiReq, true
}

// themeName returns the configured UI theme for standalone huh prompts,
// falling back to the palette default when no runtime is loaded (tests).
func themeName() string {
	if r := Runtime(); r != nil && r.Config != nil && r.Config.Config != nil {
		if name := r.Config.Config.UI.ThemeName; name != "" {
			return name
		}
	}
	return "default"
}

func newTUICommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "tui",
		Short: "Open the full-screen terminal UI",
		Long: `Open the full-screen BubbleTea terminal UI (home screen).

The TUI is strictly opt-in. Day-to-day flows stay headless or use
single-shot huh prompts: scan prints a table on non-TTY systems and
walks through selection/confirmation prompts on a TTY.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			tuiReq = &TUIRequest{Screen: "home"}
			return nil
		},
	}
	return c
}

func init() {
	rootCmd.AddCommand(newTUICommand())
}
