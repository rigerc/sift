package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go-s/internal/backend"
	"go-s/internal/model"
	planner "go-s/internal/plan"
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
	p, err := (Service{}).BuildPlan(t.TempDir(), r, nil, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 1 || p.Batches[0].Skills[0] != "one" {
		t.Fatal("auto-selected nonsuggested")
	}
	p, err = (Service{}).BuildPlan(t.TempDir(), r, []model.SkillRef{b, a}, planner.Options{})
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
		{Skill: model.SkillRef{Source: "acme/compose", Name: "docker-compose"}, URL: "https://github.com/acme/compose/tree/main/skills/docker-compose", Title: "Compose helper", SourceBackend: "fake", ExternalScore: 0.9},
		{Skill: model.SkillRef{Source: "acme/compose", Name: "plain"}, URL: "https://github.com/acme/compose/tree/main/skills/plain", SourceBackend: "fake", ExternalScore: 0.4},
	}, nil
}

// fakeRegistry builds a discovery registry containing the fake backend,
// standing in for the built-in discovery backends in tests.
func fakeRegistry(fail bool) func() (*backend.Registry, error) {
	return func() (*backend.Registry, error) {
		backend.Register("fake-search", func(cfg backend.Config) (backend.Backend, error) { return fakeSearch{fail: fail}, nil })
		return backend.NewRegistry([]backend.Config{{Name: "fake", Type: "fake-search", Capabilities: backend.CapSearch}})
	}
}

func TestScanOnlineMergesExternalSuggestions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1","novel-framework":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := Service{RegistryBuilder: fakeRegistry(false)}
	result, err := svc.Scan(context.Background(), root, ScanOptions{Online: true})
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
	if externals[0].URL != "https://github.com/acme/compose/tree/main/skills/docker-compose" {
		t.Fatalf("external GitHub URL not propagated: %+v", externals[0])
	}
	if externals[0].Reasons[0] != "matched by fake" || externals[0].Evidence[0] != "Compose helper" {
		t.Fatalf("unexpected reason/evidence: %+v", externals[0])
	}
	// Externals never enter the default plan: nil picks recommended locals only.
	plan, err := svc.BuildPlan(root, result, nil, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range plan.Batches {
		if b.Source == "acme/compose" {
			t.Fatal("external suggestion auto-selected")
		}
	}
	// Explicit external selection produces advisory npx skills arguments.
	plan, err = svc.BuildPlan(root, result, []model.SkillRef{{Source: "acme/compose", Name: "docker-compose"}}, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batches) != 1 || plan.Batches[0].Source != "acme/compose" {
		t.Fatalf("external plan not produced: %+v", plan.Batches)
	}
}

func TestScanOnlineToleratesBackendFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"1","novel-framework":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := Service{RegistryBuilder: fakeRegistry(true)}
	result, err := svc.Scan(context.Background(), root, ScanOptions{Online: true})
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

type recordingSearch struct {
	mu      sync.Mutex
	queries []backend.Query
}

func (r *recordingSearch) Name() string { return "recording" }

func (r *recordingSearch) Search(_ context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	r.mu.Lock()
	r.queries = append(r.queries, q)
	r.mu.Unlock()

	value := ""
	if len(q.Unresolved) > 0 {
		value = q.Unresolved[0].Value
	}
	shared := backend.ExternalSuggestion{
		Skill:         model.SkillRef{Source: "acme/shared", Name: "shared-helper"},
		URL:           "https://github.com/acme/shared/tree/main/skills/shared-helper",
		Title:         "Useful for multiple unresolved tools",
		SourceBackend: "recording",
		ExternalScore: 0.40,
	}
	unique := backend.ExternalSuggestion{
		Skill:         model.SkillRef{Source: "acme/unique", Name: value + "-helper"},
		URL:           "https://github.com/acme/unique/tree/main/skills/" + value + "-helper",
		SourceBackend: "recording",
		ExternalScore: 0.95,
	}
	return []backend.ExternalSuggestion{unique, shared}, nil
}

func recordingRegistry(searcher *recordingSearch) func() (*backend.Registry, error) {
	return func() (*backend.Registry, error) {
		backend.Register("recording-search", func(cfg backend.Config) (backend.Backend, error) { return searcher, nil })
		return backend.NewRegistry([]backend.Config{{Name: "recording", Type: "recording-search", Capabilities: backend.CapSearch}})
	}
}

func TestDiscoverSkipsSearchWithoutUnresolvedObservations(t *testing.T) {
	searcher := &recordingSearch{}
	svc := Service{RegistryBuilder: recordingRegistry(searcher)}
	got, warnings := svc.discover(context.Background(), nil, []model.MergedSignal{{Key: "node:react", Confidence: 1}})
	if len(got) != 0 || len(warnings) != 0 {
		t.Fatalf("context-only discovery should be skipped: got=%+v warnings=%v", got, warnings)
	}
	searcher.mu.Lock()
	defer searcher.mu.Unlock()
	if len(searcher.queries) != 0 {
		t.Fatalf("context-only discovery invoked backend: %+v", searcher.queries)
	}
}

func TestDiscoverUsesFocusedQueriesAndFusesRepeatedHits(t *testing.T) {
	searcher := &recordingSearch{}
	svc := Service{RegistryBuilder: recordingRegistry(searcher)}
	unresolved := []model.Observation{
		{Kind: model.ObsPackage, Value: "alpha", Member: "apps/a"},
		{Kind: model.ObsPackage, Value: "alpha", Member: "apps/b"},
		{Kind: model.ObsPackage, Value: "beta"},
	}
	signals := []model.MergedSignal{
		{Key: "node:react", Confidence: 0.4},
		{Key: "tool:docker", Confidence: 0.95},
		{Key: "lang:go", Confidence: 0.7},
		{Key: "docs:seo", Confidence: 0.2},
	}
	got, warnings := svc.discover(context.Background(), unresolved, signals)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(got) != 3 {
		t.Fatalf("expected fused shared + two unique results, got %+v", got)
	}
	if got[0].Skill.Name != "shared-helper" {
		t.Fatalf("result repeated across focused queries should rank first: %+v", got)
	}

	searcher.mu.Lock()
	queries := append([]backend.Query(nil), searcher.queries...)
	searcher.mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("duplicate unresolved observations should collapse to two focused queries, got %d: %+v", len(queries), queries)
	}
	for _, q := range queries {
		if len(q.Unresolved) != 1 {
			t.Fatalf("query must contain one unresolved observation: %+v", q)
		}
		if len(q.Context) != 3 || q.Context[0].Key != "tool:docker" || q.Context[1].Key != "lang:go" {
			t.Fatalf("query context was not confidence-ranked/capped: %+v", q.Context)
		}
	}
}

func TestScanDetectsNodeBuildStack(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"ogl":"1"},"devDependencies":{"electron":"1","electron-builder":"1","vite":"1","vitest":"1","typescript":"1","esbuild":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{"compilerOptions":{"strict":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vite.config.ts"), []byte(`import { defineConfig } from "vite";`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Service{}).Scan(context.Background(), root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range result.Signals {
		got[s.Key] = true
	}
	for _, want := range []string{"node:electron", "node:vite", "node:vitest", "node:typescript"} {
		if !got[want] {
			t.Fatalf("missing technology %q, got signals %+v", want, result.Signals)
		}
	}
}

func TestSortSuggestionsByScoreMixesBuckets(t *testing.T) {
	got := []model.Suggestion{
		{Skill: model.SkillRef{Source: "o/r", Name: "hidden-low"}, Bucket: "hidden", Confidence: 0.1},
		{Skill: model.SkillRef{Source: "o/r", Name: "local-mid"}, Bucket: "possible", Confidence: 0.5},
		{Skill: model.SkillRef{Source: "ext/src", Name: "external-top"}, Bucket: "external", ExternalScore: 0.9},
		{Skill: model.SkillRef{Source: "o/r", Name: "local-top"}, Bucket: "suggested", Confidence: 0.95},
		{Skill: model.SkillRef{Source: "ext/src", Name: "external-mid"}, Bucket: "external", ExternalScore: 0.5},
	}
	sortSuggestionsByScore(got)
	want := []string{"local-top", "external-top", "external-mid", "local-mid", "hidden-low"}
	for i, name := range want {
		if got[i].Skill.Name != name {
			t.Fatalf("position %d = %q, want %q (full: %+v)", i, got[i].Skill.Name, name, got)
		}
	}
}

func TestSortSuggestionsByScoreTiebreaksDeterministically(t *testing.T) {
	got := []model.Suggestion{
		{Skill: model.SkillRef{Source: "o/r", Name: "b"}, Bucket: "possible", Confidence: 0.5},
		{Skill: model.SkillRef{Source: "o/r", Name: "a"}, Bucket: "possible", Confidence: 0.5},
	}
	sortSuggestionsByScore(got)
	if got[0].Skill.Name != "a" || got[1].Skill.Name != "b" {
		t.Fatalf("equal scores must fall back to skill key order: %+v", got)
	}
}
