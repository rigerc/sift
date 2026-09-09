package app

import (
	"bytes"
	"context"
	"encoding/json"
	"go-s/internal/install"
	"go-s/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenPipeline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1","novel-framework":"2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Service{}).Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suggestions) == 0 || len(result.Unresolved) != 1 || result.Unresolved[0].Value != "novel-framework" {
		t.Fatalf("pipeline lost detection/unknown: %+v", result)
	}
	result.Root = "WORKSPACE"
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("testdata", "scan.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, data) {
		t.Fatal("scan golden changed")
	}
}

func TestPlanSelectionRestoresOrderAndExcludesExternalByDefault(t *testing.T) {
	a := model.SkillRef{Source: "a/repo", Name: "one"}
	b := model.SkillRef{Source: "b/repo", Name: "two"}
	ext := model.SkillRef{Source: "c/repo", Name: "external"}
	r := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{{Skill: a, Bucket: "suggested"}, {Skill: b, Bucket: "possible"}, {Skill: ext, Bucket: "external"}}}}
	p, err := (Service{}).PlanInstall(t.TempDir(), r, nil, install.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 1 || p.Batches[0].Skills[0] != "one" {
		t.Fatal("auto-selected nonsuggested")
	}
	p, err = (Service{}).PlanInstall(t.TempDir(), r, []model.SkillRef{b, a}, install.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 2 || p.Batches[0].Source != "a/repo" {
		t.Fatal("selection reordered resolver output")
	}
}

func TestScanCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Service{}).Scan(ctx, t.TempDir(), ScanOptions{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}
