// skillscan is a headless-first CLI: scan/status/agent/install flows run
// without a terminal UI and use single-shot huh prompts on a TTY.
// The full-screen BubbleTea TUI is strictly opt-in (`skillscan tui`,
// --tui, or scan/install --tui/--interactive).
package main

import (
	"context"
	"fmt"
	"go-s/cmd"
	"go-s/config"
	"go-s/internal/app"
	"go-s/internal/logger"
	"go-s/internal/prompt"
	"go-s/internal/ui"
	"go-s/internal/ui/screens"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/charmbracelet/x/term"
)

func main() {
	// Execute the Cobra CLI. Headless and huh flows return here; only an
	// explicit TUI request falls through to the BubbleTea program below.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Command execution failed: %v\n", err)
		os.Exit(1)
	}

	req, ok := cmd.RequestedTUI()
	if !ok || !cmd.ShouldRunUI() {
		if state := cmd.Runtime(); state != nil {
			state.Cancel()
		}
		return
	}

	runtimeState := cmd.Runtime()
	if runtimeState == nil {
		return
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "skillscan needs an interactive terminal here; use 'skillscan scan --json' for headless output")
		runtimeState.Cancel()
		return
	}
	logger.Setup(runtimeState.Config.Config.Debug)
	defer logger.Close()
	cfg, configPath := runtimeState.Config.Config, runtimeState.Config.Path

	logger.Debug("starting skillscan (debug mode enabled)")
	logger.Debug("config path: %s", configPath)

	cancel := runtimeState.Cancel
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 8192)
			n := runtime.Stack(buf, false)
			logger.Debug("panic recovered: %v\n%s", r, string(buf[:n]))
			fmt.Fprintf(os.Stderr, "\n[skillscan] crashed\npanic: %v\nstack: %s\n", r, string(buf[:n]))
			os.Exit(2)
		}
	}()

	firstRunWanted := config.IsFirstRun(configPath) && !cmd.SkipWelcome()
	logger.Debug("first run: %v", firstRunWanted)
	logger.Debug("starting UI")

	// First-run welcome uses a one-shot huh note before the TUI opens.
	// It persists only the welcome acknowledgement, never one-shot CLI flags.
	if firstRunWanted && req.Screen == "home" {
		if err := prompt.ShowWelcome(cfg.UI.ThemeName); err != nil {
			logger.Debug("welcome prompt: %v", err)
			fmt.Fprintf(os.Stderr, "welcome: %v\n", err)
			runtimeState.Cancel()
			os.Exit(1)
		}
		cfg.ConfigVersion = config.CurrentConfigVersion
		if err := config.Save(cfg, configPath); err != nil {
			logger.Debug("welcome save: %v", err)
			fmt.Fprintf(os.Stderr, "welcome save: %v\n", err)
			runtimeState.Cancel()
			os.Exit(1)
		}
	}

	services := ui.ServicesFromApp(app.Service{}, app.ScanOptions{
		Catalog: cfg.Scan.Catalog, MaxDepth: cfg.Scan.MaxDepth, Online: cfg.Scan.Online,
		ConfigDir: filepath.Dir(configPath),
	})
	model := ui.NewWithPersisted(ctx, cancel, *cfg, *runtimeState.Config.Persisted, configPath, false, services)
	switch req.Screen {
	case "scan":
		deps := ui.ServicesFromApp(app.Service{}, req.ScanOpts)
		model = ui.NewWithScreen(ctx, cancel, *cfg, *runtimeState.Config.Persisted, configPath, false, screens.NewScan(ctx, req.ScanRoot, deps), deps)
	case "install":
		deps := ui.ServicesFromApp(app.Service{}, app.ScanOptions{ConfigDir: filepath.Dir(configPath)})
		model = ui.NewWithScreen(ctx, cancel, *cfg, *runtimeState.Config.Persisted, configPath, false, screens.NewInstall(ctx, req.Plan, deps), deps)
	}
	if err := ui.Run(ctx, model); err != nil {
		logger.Debug("Program exited: %v", err)
		os.Exit(1)
	}
}
