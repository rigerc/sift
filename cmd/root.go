// Package cmd provides the CLI commands for the application using Cobra.
// This is the root command that all subcommands are attached to.
package cmd

import (
	"context"
	"go-s/config"

	"github.com/spf13/cobra"
)

var (
	// cfgFile holds the path to the configuration file.
	cfgFile string

	// debugMode indicates if debug mode is enabled.
	debugMode bool

	// skipWelcome suppresses the first-run welcome screen.
	skipWelcome bool

	// logLevel sets the logging verbosity.
	logLevel string

	// tuiFlag forces full-screen TUI after scan/install subcommands.
	tuiFlag bool

	// runUI indicates whether to run the full-screen TUI after command execution.
	// The TUI is strictly opt-in: bare root and headless/huh flows leave it false.
	// Only `tui`, --tui, or scan/install --tui/--interactive set it.
	runUI        = false
	runtimeState *RuntimeState
)

type RuntimeState struct {
	Context context.Context
	Cancel  context.CancelFunc
	Config  *config.EffectiveConfig
}

var processContext context.Context = context.Background()

func SetContext(ctx context.Context) { processContext = ctx }
func Runtime() *RuntimeState         { return runtimeState }

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "skillscan",
	Short: "Scan workspaces and suggest agent skills",
	Long: `skillscan scans a workspace for technologies and suggests installable agent skills.

The CLI is headless-first: scan prints a table (or JSON), and on an
interactive terminal it walks through huh selection/confirmation prompts.
The full-screen BubbleTea TUI is strictly opt-in via "skillscan tui".`,
	Example: `  # Headless scan of the current workspace
  skillscan scan

  # Interactive selection on a TTY (huh prompts, no TUI)
  skillscan scan .

  # JSON output / dry-run install plan
  skillscan scan --json
  skillscan scan --dry-run

  # Opt-in full-screen TUI
  skillscan tui

  # Show version information
  skillscan version`,
	Version: "1.0.0",
	// Run executes the root command.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bare invocation prints a headless scan hint instead of opening the TUI.
		// Run `skillscan tui` (or pass --tui) for the full-screen interface.
		return cmd.Help()
	},
}

// Execute runs the root command. This is called from main.go.
// It returns an error if the command fails.
// The full-screen TUI is strictly opt-in: only `tui`, --tui, or
// scan/install --tui/--interactive set runUI. Everything else (bare root,
// headless and huh flows, help/version output) leaves the TUI off.
func Execute() error {
	runUI = false
	tuiReq = nil
	runtimeState = nil
	rootCmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		if c.Name() == "version" || c.Name() == "completion" {
			return nil
		}
		debug := debugMode
		level := logLevel
		overrides := config.RuntimeOverrides{}
		if rootCmd.PersistentFlags().Changed("debug") {
			overrides.Debug = &debug
		}
		if rootCmd.PersistentFlags().Changed("log-level") {
			overrides.LogLevel = &level
		}
		effective, err := config.LoadEffective(GetConfigFile(), cfgFile != "", overrides)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(processContext)
		runtimeState = &RuntimeState{Context: ctx, Cancel: cancel, Config: effective}
		return nil
	}
	c, err := rootCmd.ExecuteC()
	if err != nil {
		runUI = false
		return err
	}
	if c.Name() == "version" || c.Name() == "completion" {
		runUI = false
		return nil
	}
	if tuiFlag {
		runUI = true
		if tuiReq == nil {
			tuiReq = &TUIRequest{Screen: "home"}
		}
		return nil
	}
	if tuiReq != nil {
		// Explicit `tui` subcommand (or scan/install --tui/--interactive).
		runUI = true
		return nil
	}
	// Bare root, help/version output, headless and huh flows: no TUI.
	runUI = false
	return nil
}

// GetRootCmd returns the root Cobra command.
// This allows the main package to access command configuration
// before executing the TUI application.
func GetRootCmd() *cobra.Command {
	return rootCmd
}

// IsDebugMode returns whether debug mode is enabled.
// This can be checked anywhere in the codebase to enable
// additional logging or debugging features.
func IsDebugMode() bool {
	return debugMode
}

// ShouldRunUI returns whether the full-screen TUI should be run after
// command execution. True only for the opt-in `tui` command / --tui flag.
func ShouldRunUI() bool {
	return runUI
}

// init initializes the root command with flags and configuration.
func init() {
	// Config file flag
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Path to configuration file (default: $XDG_CONFIG_HOME/a-go-s/config.json; derived from the legacy App.Name)")

	// Debug mode flag
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false,
		"Enable debug mode with trace logging")

	// Skip welcome screen flag
	rootCmd.PersistentFlags().BoolVar(&skipWelcome, "skip-welcome", false,
		"Skip the first-run welcome screen")

	// Log level flag
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Set logging level (trace, debug, info, warn, error, fatal)")

	// TUI opt-in flag. Headless table/JSON output and standalone huh
	// prompts are the default; this forces the full-screen BubbleTea UI.
	rootCmd.PersistentFlags().BoolVar(&tuiFlag, "tui", false,
		"Open the full-screen terminal UI instead of headless/huh output")
}

// GetConfigFile returns the path to the configuration file, computing default if needed.
func GetConfigFile() string {
	if cfgFile != "" {
		return cfgFile
	}
	return config.DefaultConfigPath()
}

// GetLogLevel returns the configured log level.
func GetLogLevel() string {
	return logLevel
}

// SkipWelcome reports whether the --skip-welcome flag was passed.
func SkipWelcome() bool {
	return skipWelcome
}

// WasLogLevelSet reports whether --log-level was explicitly passed on the command line.
// Use this to distinguish an explicit flag from Cobra's default value.
func WasLogLevelSet() bool {
	return rootCmd.PersistentFlags().Changed("log-level")
}
