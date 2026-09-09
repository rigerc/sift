package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBackendsPrecedenceAndAuthReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backends.yaml")
	if err := os.WriteFile(path, []byte("strategy: first-hit\nbackends:\n  - name: local\n    type: catalog\n    url: file://rules.yaml\n    auth: env:TEST_BACKEND_TOKEN\n    capabilities: [catalog]\n    timeout: 2s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("SKILLSCAN_BACKENDS", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("SKILLSCAN_BACKENDS") })
	cfg, selected, err := LoadBackends("", filepath.Join(dir, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if selected != path || cfg.Strategy != "first-hit" || cfg.Backends[0].Timeout.String() != "2s" {
		t.Fatalf("config=%#v path=%q", cfg, selected)
	}
	if _, err := ResolveBackendAuth("env:TEST_BACKEND_TOKEN"); err == nil {
		t.Fatal("expected missing auth env error")
	}
}
