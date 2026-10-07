package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go-s/internal/app"
	"go-s/internal/install"
	"go-s/internal/report"
)

func TestHeadlessScanMatchesService(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newScanCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{root, "--json"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got report.ScanEnvelope
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("machine output contaminated: %s: %v", out.String(), err)
	}
	want, err := (app.Service{}).Scan(context.Background(), root, app.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(report.BuildScan(want, report.ScanOptions{}))
	if !bytes.Equal(a, b) {
		t.Fatal("CLI and service disagree")
	}
}

func TestInstallDryRunNoNode(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := newInstallCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"owner/repo", "--skill", "test", "--agent", "github-copilot", "--dry-run"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) || !bytes.Contains(out.Bytes(), []byte("\"skills\"")) {
		t.Fatal(out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("skills@")) {
		t.Fatalf("skills CLI must be unpinned: %s", out.String())
	}
}

func TestAgentAlwaysHeadless(t *testing.T) {
	root := t.TempDir()
	c := newAgentCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{root, "--json", "--no-instructions"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("invalid envelope: %s", out.String())
	}
}

func TestValidatePlanOfflineRequiresNothing(t *testing.T) {
	p := install.Plan{Batches: []install.Batch{{Source: "owner/repo", Skills: []string{"x"}}}}
	if err := validatePlan(context.Background(), p, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlanOnlineDegradesWithoutValidators(t *testing.T) {
	// The built-in registry ships search-only backends, so online validation
	// is a no-op instead of a hard failure.
	p := install.Plan{Batches: []install.Batch{{Source: "owner/repo", Skills: []string{"x"}}}}
	if err := validatePlan(context.Background(), p, true, false); err != nil {
		t.Fatalf("built-in registry must degrade gracefully: %v", err)
	}
}

func TestBackendsListShipsBuiltins(t *testing.T) {
	c := newBackendsCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"list", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 built-in backends: %s", out.String())
	}
	want := []string{"official-skills", "skyll", "skillsmp", "decimalai"}
	for i, name := range want {
		if rows[i]["Name"] != name || rows[i]["Type"] != name {
			t.Fatalf("unexpected backend at %d, want %q: %s", i, name, out.String())
		}
		if rows[i]["Capabilities"] != "search" {
			t.Fatalf("backend %s is search-only: %s", name, out.String())
		}
	}
}
