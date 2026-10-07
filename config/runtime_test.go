package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEffectivePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"scan":{"catalog":"file","online":true,"maxDepth":16},"install":{"global":true}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIFT_CATALOG_URL", "environment")
	// Removed environment settings must have no effect or validation.
	t.Setenv("SKILLSCAN_DEBUG", "invalid")
	t.Setenv("SKILLSCAN_LOG_LEVEL", "invalid")
	env, err := LoadEffective(path, true, RuntimeOverrides{})
	if err != nil || env.Config.Scan.Catalog != "environment" {
		t.Fatalf("%+v %v", env, err)
	}
	catalog, depth, no := "flag", 3, false
	got, err := LoadEffective(path, true, RuntimeOverrides{Catalog: &catalog, MaxDepth: &depth, Online: &no, Global: &no})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Scan.Catalog != "flag" || got.Config.Scan.MaxDepth != 3 || got.Config.Scan.Online || got.Config.Install.Global {
		t.Fatalf("flags did not win: %+v", got.Config)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("configuration modified")
	}
}

func TestMissingAndMalformedPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := DefaultConfigPath()
	if _, err := LoadEffective(path, true, RuntimeOverrides{}); !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("missing explicit: %v", err)
	}
	got, err := LoadEffective(path, false, RuntimeOverrides{})
	if err != nil || got.Config.Scan.MaxDepth != 8 || !got.Config.Scan.Online || got.Config.Install.Global {
		t.Fatalf("%+v %v", got, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, explicit := range []bool{false, true} {
		if _, err := LoadEffective(path, explicit, RuntimeOverrides{}); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("malformed: %v", err)
		}
	}
}

func TestLegacyCatalogEnvironmentIgnored(t *testing.T) {
	t.Setenv("SIFT_CATALOG_URL", "")
	if err := os.Unsetenv("SIFT_CATALOG_URL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKILLSCAN_CATALOG_URL", "retired")
	got, err := LoadEffective(filepath.Join(t.TempDir(), "missing"), false, RuntimeOverrides{})
	if err != nil || got.Config.Scan.Catalog != "" {
		t.Fatalf("retired environment setting applied: %+v %v", got, err)
	}
}

func TestRuntimeDepthRejectsZero(t *testing.T) {
	for _, depth := range []int{0, -1, 65} {
		if _, err := LoadEffective(filepath.Join(t.TempDir(), "missing"), false, RuntimeOverrides{MaxDepth: &depth}); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("runtime depth %d accepted: %v", depth, err)
		}
	}
}
