package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func resetCLIState(t *testing.T) {
	t.Helper()
	resetState()
	t.Cleanup(resetState)
}

func resetState() {
	cfgFile = ""
	debugMode = false
	skipWelcome = false
	logLevel = "info"
	tuiFlag = false
	runUI = false
	tuiReq = nil
	if runtimeState != nil {
		runtimeState.Cancel()
		runtimeState = nil
	}
	// Subcommand objects are shared across Execute calls, so reset every
	// flag to its default to avoid value leakage between tests.
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	for _, sub := range rootCmd.Commands() {
		resetSubFlags(sub)
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func resetSubFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	for _, sub := range c.Commands() {
		resetSubFlags(sub)
	}
}

func executeRoot(t *testing.T, args ...string) error {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	return Execute()
}

func TestBareRootRequestsNoTUI(t *testing.T) {
	resetCLIState(t)
	if err := executeRoot(t); err != nil {
		t.Fatalf("bare root: %v", err)
	}
	if ShouldRunUI() {
		t.Fatal("bare root must not request the TUI")
	}
	if _, ok := RequestedTUI(); ok {
		t.Fatal("bare root must not leave a TUI request")
	}
}

func TestVersionRequestsNoTUI(t *testing.T) {
	resetCLIState(t)
	if err := executeRoot(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if ShouldRunUI() {
		t.Fatal("version must not request the TUI")
	}
}

func TestTUICommandRequestsHome(t *testing.T) {
	resetCLIState(t)
	if err := executeRoot(t, "tui"); err != nil {
		t.Fatalf("tui: %v", err)
	}
	if !ShouldRunUI() {
		t.Fatal("tui command must request the TUI")
	}
	req, ok := RequestedTUI()
	if !ok || req.Screen != "home" {
		t.Fatalf("expected home TUI request, got %#v", req)
	}
}

func TestScanTUIFlagRequestsScanScreen(t *testing.T) {
	resetCLIState(t)
	root := t.TempDir()
	if err := executeRoot(t, "scan", "--tui", root); err != nil {
		t.Fatalf("scan --tui: %v", err)
	}
	if !ShouldRunUI() {
		t.Fatal("scan --tui must request the TUI")
	}
	req, ok := RequestedTUI()
	if !ok || req.Screen != "scan" || req.ScanRoot != root {
		t.Fatalf("expected scan TUI request, got %#v", req)
	}
}

func TestPersistentTUIFlagRoutesThroughScan(t *testing.T) {
	resetCLIState(t)
	root := t.TempDir()
	if err := executeRoot(t, "--tui", "scan", root); err != nil {
		t.Fatalf("--tui scan: %v", err)
	}
	req, ok := RequestedTUI()
	if !ok || req.Screen != "scan" {
		t.Fatalf("persistent --tui must keep the scan screen, got %#v", req)
	}
}

func TestScanInteractiveConflictsWithJSON(t *testing.T) {
	resetCLIState(t)
	root := t.TempDir()
	err := executeRoot(t, "scan", "--interactive", "--json", root)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if ShouldRunUI() {
		t.Fatal("conflicting scan flags must not request the TUI")
	}
}

func TestInstallTUIConflictsWithYes(t *testing.T) {
	resetCLIState(t)
	err := executeRoot(t, "install", "owner/repo", "--skill", "x", "--tui", "--yes")
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestScanJSONRequestsNoTUI(t *testing.T) {
	resetCLIState(t)
	root := t.TempDir()
	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"scan", root, "--json"})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	if err := Execute(); err != nil {
		t.Fatalf("scan --json: %v", err)
	}
	if ShouldRunUI() {
		t.Fatal("scan --json must not request the TUI")
	}
	if _, ok := RequestedTUI(); ok {
		t.Fatal("scan --json must not leave a TUI request")
	}
}
