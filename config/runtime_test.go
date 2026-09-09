package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEffectivePrecedenceAndExplicitFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"debug":true,"logLevel":"warn"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKILLSCAN_DEBUG", "true")
	t.Setenv("SKILLSCAN_LOG_LEVEL", "error")
	debug := false
	level := "trace"
	effective, err := LoadEffective(path, true, RuntimeOverrides{Debug: &debug, LogLevel: &level})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Debug || effective.Config.LogLevel != "trace" {
		t.Fatalf("runtime flags did not win: %+v", effective.Config)
	}
	if !effective.Persisted.Debug || effective.Persisted.LogLevel != "warn" {
		t.Fatalf("persisted values were mutated: %+v", effective.Persisted)
	}
}

func TestLoadEffectiveMissingExplicitFileFails(t *testing.T) {
	_, err := LoadEffective(filepath.Join(t.TempDir(), "missing.json"), true, RuntimeOverrides{})
	if err == nil {
		t.Fatal("expected missing explicit config to fail")
	}
}

func TestLoadEffectiveMissingDefaultFileIsValid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	effective, err := LoadEffective(DefaultConfigPath(), false, RuntimeOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", effective.Config)
	}
}
