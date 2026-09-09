package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"go-s/internal/app"
	"go-s/internal/install"
	"go-s/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	var got model.ScanResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("machine output contaminated: %s: %v", out.String(), err)
	}
	want, err := (app.Service{}).Scan(context.Background(), root, app.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
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
	if !json.Valid(out.Bytes()) || !bytes.Contains(out.Bytes(), []byte("skills@1.5.25")) {
		t.Fatal(out.String())
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
	if err := validatePlan(context.Background(), p, app.ScanOptions{}, false); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlanOnlineUnknownRequiresFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backends.yaml")
	yaml := "strategy: fanout\nbackends:\n  - name: gh\n    type: github-trees\n    url: http://127.0.0.1:1\n    capabilities: [validate]\n    timeout: 1s\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p := install.Plan{Batches: []install.Batch{{Source: "owner/repo", Skills: []string{"x"}}}}
	opts := app.ScanOptions{Online: true, BackendsPath: path}
	if err := validatePlan(context.Background(), p, opts, false); err == nil || !strings.Contains(err.Error(), "--allow-unvalidated") {
		t.Fatalf("expected unknown gate, got %v", err)
	}
	if err := validatePlan(context.Background(), p, opts, true); err != nil {
		t.Fatal(err)
	}
}

func TestBackendsList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backends.yaml")
	yaml := "strategy: fanout\nbackends:\n  - name: ss\n    type: skills.sh\n    capabilities: [search, validate]\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newBackendsCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"list", "--backends", path, "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["Name"] != "ss" || rows[0]["Type"] != "skills.sh" || rows[0]["Capabilities"] != "search,validate" {
		t.Fatalf("unexpected rows: %s", out.String())
	}
}
