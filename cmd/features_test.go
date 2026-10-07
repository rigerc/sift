package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rigerc/sift/internal/app"
	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/plan"
	"github.com/rigerc/sift/internal/report"

	"github.com/spf13/cobra"
)

func testRoot(t *testing.T) *cobra.Command {
	t.Helper()
	oldConfig, oldRuntime := cfgFile, runtimeState
	cfgFile, runtimeState = "", nil
	t.Cleanup(func() { cfgFile, runtimeState = oldConfig, oldRuntime })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SIFT_CATALOG_URL", "")
	c := newRootCommand()
	c.AddCommand(newScanCommand(), newPlanCommand(), newAgentCommand(), newBackendsCommand())
	version, completion := *versionCmd, *completionCmd
	c.AddCommand(&version, &completion)
	return c
}

func execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	c := testRoot(t)
	var out, chrome bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&chrome)
	c.SetArgs(args)
	err := c.ExecuteContext(context.Background())
	return out.String(), chrome.String(), err
}

func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScanJSONMatchesServiceAndBypassesTTY(t *testing.T) {
	oldTTY, oldSelect := ttyDetected, selectSkills
	ttyDetected = func() bool { t.Fatal("machine mode checked terminal"); return true }
	selectSkills = func(context.Context, model.ScanResult) ([]model.SkillRef, error) {
		t.Fatal("machine mode prompted")
		return nil, nil
	}
	t.Cleanup(func() { ttyDetected, selectSkills = oldTTY, oldSelect })
	root := workspace(t)
	out, chrome, err := execute(t, "scan", root, "--json", "--online=false")
	if err != nil {
		t.Fatal(err)
	}
	if chrome != "" {
		t.Fatalf("unexpected chrome: %q", chrome)
	}
	var got report.ScanEnvelope
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("contaminated output: %s: %v", out, err)
	}
	want, err := (app.Service{}).Scan(context.Background(), root, app.ScanOptions{MaxDepth: 8, Online: false})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(report.BuildScan(want, report.ScanOptions{}))
	if !bytes.Equal(a, b) {
		t.Fatal("CLI and service disagree")
	}

	out, _, err = execute(t, "scan", root, "--dry-run", "--online=false", "--agent", "codex", "--global")
	if err != nil {
		t.Fatal(err)
	}
	var gotPlan plan.Plan
	if err := json.Unmarshal([]byte(out), &gotPlan); err != nil {
		t.Fatal(err)
	}
	wantPlan, err := (app.Service{}).BuildPlan(root, want, nil, plan.Options{Agents: []string{"codex"}, Global: true, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotPlan, wantPlan) {
		t.Fatalf("dry-run differs: %+v != %+v", gotPlan, wantPlan)
	}
}

func TestScanHeadlessTableAndEmptyDryRun(t *testing.T) {
	oldTTY := ttyDetected
	ttyDetected = func() bool { return false }
	t.Cleanup(func() { ttyDetected = oldTTY })
	root := workspace(t)
	out, _, err := execute(t, "scan", root, "--online=false")
	if err != nil {
		t.Fatal(err)
	}
	result, err := (app.Service{}).Scan(context.Background(), root, app.ScanOptions{MaxDepth: 8})
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := report.Table(&want, result, false); err != nil {
		t.Fatal(err)
	}
	if out != want.String() {
		t.Fatalf("not a headless table: %s", out)
	}
	out, _, err = execute(t, "scan", t.TempDir(), "--dry-run", "--online=false")
	if err != nil {
		t.Fatal(err)
	}
	var p plan.Plan
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	if p.Batches == nil || len(p.Batches) != 0 {
		t.Fatalf("empty plan must have valid [] batches: %+v", p)
	}
}

func TestTTYSelectionPlanCancellationAndEmpty(t *testing.T) {
	oldTTY, oldSelect := ttyDetected, selectSkills
	ttyDetected = func() bool { return true }
	t.Cleanup(func() { ttyDetected, selectSkills = oldTTY, oldSelect })
	root := workspace(t)
	selectSkills = func(_ context.Context, result model.ScanResult) ([]model.SkillRef, error) {
		for _, s := range result.Suggestions {
			if s.Bucket == "suggested" {
				return []model.SkillRef{s.Skill}, nil
			}
		}
		t.Fatal("fixture has no recommended skills")
		return nil, nil
	}
	out, _, err := execute(t, "scan", root, "--online=false")
	if err != nil || !strings.Contains(out, "Plan only—nothing installed") || !strings.Contains(out, "npx ") {
		t.Fatalf("%s %v", out, err)
	}
	selectSkills = func(context.Context, model.ScanResult) ([]model.SkillRef, error) { return nil, nil }
	out, _, err = execute(t, "scan", root, "--online=false")
	if err != nil || out != "" {
		t.Fatalf("cancel/empty emitted plan: %q %v", out, err)
	}
	selectSkills = func(context.Context, model.ScanResult) ([]model.SkillRef, error) {
		return []model.SkillRef{{Source: "owner/repo", Name: "partial"}}, context.Canceled
	}
	out, _, err = execute(t, "scan", root, "--online=false")
	if !errors.Is(err, context.Canceled) || out != "" {
		t.Fatalf("partial cancellation emitted output: %q %v", out, err)
	}
}

func TestScanConflictsAndDepthFailBeforeScan(t *testing.T) {
	for _, args := range [][]string{
		{"scan", "/missing", "--json", "--dry-run"},
		{"scan", "/missing", "--max-depth=0"},
		{"scan", "/missing", "--max-depth=-1"},
		{"scan", "/missing", "--max-depth=65"},
		{"agent", "/missing", "--max-depth=0"},
	} {
		out, _, err := execute(t, args...)
		if err == nil || out != "" || (!strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "max-depth")) {
			t.Fatalf("%v: %q %v", args, out, err)
		}
	}
}

func TestExplicitPlanTextJSONAndNoExecutionOrTracking(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	paths := []string{"skill-lock.json", ".agents/skills/keep/SKILL.md", ".claude/skills/keep/SKILL.md"}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("malformed legacy content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, jsonMode := range []bool{false, true} {
		args := []string{"plan", "owner/repo", "--skill", "test", "--agent", "github-copilot", "--global"}
		if jsonMode {
			args = append(args, "--json")
		}
		out, chrome, err := execute(t, args...)
		if err != nil || chrome != "" {
			t.Fatalf("%s %v", chrome, err)
		}
		if jsonMode {
			var p plan.Plan
			if err := json.Unmarshal([]byte(out), &p); err != nil {
				t.Fatal(err)
			}
			if p.Scope != "global" || !reflect.DeepEqual(p.Agents, []string{"github-copilot"}) || len(p.Batches) != 1 || p.Batches[0].Source != "owner/repo" {
				t.Fatalf("%+v", p)
			}
		} else if !strings.Contains(out, "Plan only—nothing installed") || !strings.Contains(out, "npx ") || !strings.Contains(out, "--global") {
			t.Fatal(out)
		}
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "malformed legacy content" {
			t.Fatalf("changed %s", path)
		}
	}
	if _, err := os.Stat(".skillsense"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("tracking state created")
	}
}

func TestPlanInvalidReferencesEmitNothing(t *testing.T) {
	for _, args := range [][]string{
		{"plan", "owner/repo"},
		{"plan", "--", "--evil"},
		{"plan", "owner/repo", "--skill", "--flag"},
		{"plan", "owner/repo", "--skill", "../escape"},
		{"plan", "owner/repo\n", "--skill", "x"},
		{"plan", "https://user:pass@github.com/o/r", "--skill", "x"},
		{"plan", "owner/repo", "--skill", "x", "--agent", "unknown"},
	} {
		out, _, err := execute(t, args...)
		if err == nil || out != "" {
			t.Fatalf("%v emitted instructions: %q %v", args, out, err)
		}
	}
}

func TestAgentAlwaysHeadless(t *testing.T) {
	oldSelect := selectSkills
	selectSkills = func(context.Context, model.ScanResult) ([]model.SkillRef, error) {
		t.Fatal("agent prompted")
		return nil, nil
	}
	t.Cleanup(func() { selectSkills = oldSelect })
	out, _, err := execute(t, "agent", t.TempDir(), "--json", "--no-instructions", "--online=false")
	if err != nil || !json.Valid([]byte(out)) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestBackendsListShipsFourSearchProviders(t *testing.T) {
	out, _, err := execute(t, "backends", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	want := []string{"official-skills", "skyll", "skillsmp", "decimalai"}
	if len(rows) != len(want) {
		t.Fatal(out)
	}
	for i, name := range want {
		if rows[i]["Name"] != name || rows[i]["Type"] != name || rows[i]["Capabilities"] != "search" {
			t.Fatal(out)
		}
	}
}
