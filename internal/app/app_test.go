package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go-s/internal/backend"
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

type fakeSearch struct{ fail bool }

func (f fakeSearch) Name() string { return "fake" }

func (f fakeSearch) Search(_ context.Context, _ backend.Query) ([]backend.ExternalSuggestion, error) {
	if f.fail {
		return nil, errors.New("backend down")
	}
	return []backend.ExternalSuggestion{
		{Skill: model.SkillRef{Source: "askill", Name: "docker-compose"}, Title: "Compose helper", SourceBackend: "fake", ExternalScore: 0.9},
		{Skill: model.SkillRef{Source: "askill", Name: "plain"}, SourceBackend: "fake", ExternalScore: 0.4},
	}, nil
}

func writeBackendsConfig(t *testing.T, dir string) {
	t.Helper()
	body := "strategy: fanout\nbackends:\n  - name: fake\n    type: fake-search\n    capabilities: [search]\n"
	if err := os.WriteFile(filepath.Join(dir, "backends.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanOnlineMergesExternalSuggestions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	writeBackendsConfig(t, configDir)
	// Registration happens in the injected builder; use the real registry.
	svc := Service{RegistryBuilder: backend.NewRegistry}
	backend.Register("fake-search", func(cfg backend.Config) (backend.Backend, error) { return fakeSearch{}, nil })
	result, err := svc.Scan(context.Background(), root, ScanOptions{Online: true, ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	var externals []model.Suggestion
	for _, s := range result.Suggestions {
		if s.Bucket == "external" {
			externals = append(externals, s)
		}
	}
	if len(externals) != 2 {
		t.Fatalf("want 2 external suggestions, got %+v", result.Suggestions)
	}
	if externals[0].Skill.Name != "docker-compose" || externals[0].ExternalScore != 0.9 || externals[0].SourceBackend != "fake" {
		t.Fatalf("external ordering/mapping broken: %+v", externals[0])
	}
	if externals[0].Reasons[0] != "matched by fake" || externals[0].Evidence[0] != "Compose helper" {
		t.Fatalf("unexpected reason/evidence: %+v", externals[0])
	}
	// Externals never auto-install: nil selection picks suggested only.
	plan, err := svc.PlanInstall(root, result, nil, install.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range plan.Batches {
		if b.Source == "askill" {
			t.Fatal("external suggestion auto-selected")
		}
	}
	// Explicit selection of an external installs through the backend CLI.
	plan, err = svc.PlanInstall(root, result, []model.SkillRef{{Source: "askill", Name: "docker-compose"}}, install.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batches) != 1 || plan.Batches[0].Backend != "askill" {
		t.Fatalf("backend plan not produced: %+v", plan.Batches)
	}
}

func TestScanOnlineToleratesBackendFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	writeBackendsConfig(t, configDir)
	backend.Register("fake-search", func(cfg backend.Config) (backend.Backend, error) { return fakeSearch{fail: true}, nil })
	svc := Service{RegistryBuilder: backend.NewRegistry}
	result, err := svc.Scan(context.Background(), root, ScanOptions{Online: true, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("backend failure must not fail the scan: %v", err)
	}
	found := false
	for _, w := range result.Warnings {
		if bytes.Contains([]byte(w), []byte("backend down")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("backend failure not reported as warning: %v", result.Warnings)
	}
}
