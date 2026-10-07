package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rigerc/sift/config"
)

func TestBareHelpVersionAndRemovedSurface(t *testing.T) {
	out, _, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sift scan", "sift plan", "$XDG_CONFIG_HOME/sift/config.json", "nothing is installed", "Run printed npx skills commands yourself", "Available Commands:"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q: %s", want, out)
		}
	}
	out, _, err = execute(t, "version")
	if err != nil || out != "sift v1.0.0\n" {
		t.Errorf("%q %v", out, err)
	}
	for _, command := range []string{"tui", "install", "status", "update"} {
		out, _, err = execute(t, command)
		if err == nil || !strings.Contains(err.Error(), "unknown command") || out != "" {
			t.Fatalf("obsolete %s accepted: %q %v", command, out, err)
		}
	}
	for _, flag := range []string{"tui", "interactive", "yes", "allow-unvalidated", "narsil", "narsil-path", "skip-welcome", "debug", "log-level"} {
		for _, command := range []string{"scan", "plan"} {
			out, _, err = execute(t, command, "--"+flag)
			if err == nil || !strings.Contains(err.Error(), "unknown flag") || out != "" {
				t.Fatalf("obsolete %s flag accepted by %s: %q %v", flag, command, out, err)
			}
		}
	}
	c := testRoot(t)
	var names []string
	for _, sub := range c.Commands() {
		names = append(names, sub.Name())
	}
	want := []string{"agent", "backends", "completion", "plan", "scan", "version"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("command surface %v", names)
	}
}

func TestSiftCompletionIdentity(t *testing.T) {
	out, _, err := execute(t, "completion", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sift completion") || strings.Contains(out, "skillscan") {
		t.Errorf("completion help identity: %s", out)
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out, _, err := execute(t, "completion", shell)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "sift") || strings.Contains(out, "skillscan") {
				t.Fatalf("completion identity: %s", out)
			}
		})
	}
}

func TestSiftCatalogWithoutRuntime(t *testing.T) {
	c := testRoot(t)
	t.Setenv("SIFT_CATALOG_URL", "file://environment.json")
	var f scanFlags
	f.bind(c)
	got, err := f.options(c)
	if err != nil || got.Catalog != "environment.json" {
		t.Fatalf("environment without runtime: %+v %v", got, err)
	}
}

func TestCompletionOnlyAdvertisesRetainedFlags(t *testing.T) {
	out, _, err := execute(t, "completion", "bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"--tui", "--interactive", "--allow-unvalidated", "--narsil", "--skip-welcome", "--debug", "--log-level"} {
		if strings.Contains(out, removed) {
			t.Fatalf("completion advertises %s", removed)
		}
	}
	if !strings.Contains(out, "--skill") || !strings.Contains(out, "--dry-run") {
		t.Fatal("missing planning completions")
	}
}

func TestRootConfigErrorsAndReadOnly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	out, _, err := execute(t, "--config", missing, "version")
	if err == nil || !strings.Contains(err.Error(), "configuration file not found") || out != "" {
		t.Fatalf("missing explicit config: %q %v", out, err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err = execute(t, "--config", path, "plan", "owner/repo", "--skill", "x")
	if err == nil || out != "" {
		t.Fatalf("malformed config: %q %v", out, err)
	}
	data := []byte(`{"scan":{"maxDepth":0,"online":false},"install":{"global":true},"ui":{"themeName":"legacy"}}`)
	if err := os.WriteFile(path, data, 0o444); err != nil {
		t.Fatal(err)
	}
	out, _, err = execute(t, "--config", path, "plan", "owner/repo", "--skill", "x")
	if err != nil || !strings.Contains(out, "Scope: global") {
		t.Fatalf("%q %v", out, err)
	}
	out, _, err = execute(t, "--config", path, "plan", "owner/repo", "--skill", "x", "--global=false")
	if err != nil || !strings.Contains(out, "Scope: project") {
		t.Fatalf("explicit false lost: %q %v", out, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("config was rewritten")
	}
}

func TestScanOptionsPrecedenceAndDefaults(t *testing.T) {
	c := testRoot(t)
	t.Setenv("SIFT_CATALOG_URL", "file://environment.json")
	cfg := config.DefaultConfig()
	cfg.Scan = config.ScanConfig{Catalog: "file.json", MaxDepth: 16, Online: false}
	runtimeState = &RuntimeState{Context: context.Background(), Config: &config.EffectiveConfig{Config: cfg}}
	var f scanFlags
	f.bind(c)
	got, err := f.options(c)
	// Runtime values have already applied the environment; unchanged flags
	// must not substitute their Cobra defaults (especially online=true).
	if err != nil || got.Catalog != "file.json" || got.MaxDepth != 16 || got.Online {
		t.Fatalf("%+v %v", got, err)
	}
	if err := c.ParseFlags([]string{"--catalog=file://flag.json", "--max-depth=1", "--online=true"}); err != nil {
		t.Fatal(err)
	}
	got, err = f.options(c)
	if err != nil || got.Catalog != "flag.json" || got.MaxDepth != 1 || !got.Online {
		t.Fatalf("%+v %v", got, err)
	}
	if err := c.Flags().Set("online", "false"); err != nil {
		t.Fatal(err)
	}
	got, err = f.options(c)
	if err != nil || got.Online {
		t.Fatalf("explicit false: %+v %v", got, err)
	}

	runtimeState = nil
	fresh := testRoot(t)
	var defaults scanFlags
	defaults.bind(fresh)
	got, err = defaults.options(fresh)
	if err != nil || got.MaxDepth != 8 || !got.Online {
		t.Fatalf("defaults: %+v %v", got, err)
	}
}

func TestExecuteCleansRuntimeAfterFailure(t *testing.T) {
	oldRoot, oldContext := rootCmd, processContext
	rootCmd = testRoot(t)
	t.Cleanup(func() { rootCmd, processContext = oldRoot, oldContext })
	SetContext(context.Background())
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"plan", "owner/repo"})
	if err := Execute(); err == nil {
		t.Fatal("missing skill accepted")
	}
	if Runtime() != nil {
		t.Fatal("runtime retained after command failure")
	}
}
