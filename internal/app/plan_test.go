package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go-s/internal/backend"
	"go-s/internal/model"
	"go-s/internal/plan"
)

func TestBuildPlanDefaultSelectionRequiresRecommendedLocalCatalog(t *testing.T) {
	local := model.SkillRef{Source: "catalog/repo", Name: "recommended"}
	r := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
		{Skill: model.SkillRef{Source: "external/repo", Name: "external"}, Bucket: "external"},
		// Backend provenance cannot become implicit consent through a mislabeled bucket.
		{Skill: model.SkillRef{Source: "external/repo", Name: "mislabeled"}, Bucket: "suggested", SourceBackend: "skyll"},
		{Skill: model.SkillRef{Source: "catalog/repo", Name: "possible"}, Bucket: "possible"},
		{Skill: model.SkillRef{Source: "catalog/repo", Name: "hidden"}, Bucket: "hidden"},
		{Skill: local, Bucket: "suggested"},
	}}}
	p, err := (Service{}).BuildPlan(t.TempDir(), r, nil, plan.Options{})
	if err != nil || len(p.Batches) != 1 || !reflect.DeepEqual(p.Batches[0].Skills, []string{local.Name}) {
		t.Fatalf("default selection is not recommended-local only: %+v %v", p, err)
	}
	p, err = (Service{}).BuildPlan(t.TempDir(), r, []model.SkillRef{}, plan.Options{})
	if err != nil || p.Batches == nil || len(p.Batches) != 0 {
		t.Fatalf("explicit empty selection must remain empty: %+v %v", p, err)
	}
	r.Suggestions = r.Suggestions[:4]
	p, err = (Service{}).BuildPlan(t.TempDir(), r, nil, plan.Options{})
	if err != nil || p.Batches == nil || len(p.Batches) != 0 {
		t.Fatalf("no recommendations must yield a valid empty plan: %+v %v", p, err)
	}
}

func TestBuildPlanSelectionPreservesScanOrderAndRejectsUnknownOrHostile(t *testing.T) {
	first := model.SkillRef{Source: "one/repo", Name: "first"}
	second := model.SkillRef{Source: "two/repo", Name: "second"}
	ext := model.SkillRef{Source: "https://skills.example/third", Name: "third"}
	r := model.ScanResult{ResolveResult: model.ResolveResult{
		Suggestions: []model.Suggestion{{Skill: first, Bucket: "suggested"}, {Skill: second, Bucket: "possible"}, {Skill: ext, Bucket: "external"}},
	}}
	p, err := (Service{}).BuildPlan(t.TempDir(), r, []model.SkillRef{ext, second, first, second}, plan.Options{Agents: []string{"codex"}, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{first.Source, second.Source, ext.Source}
	if p.Scope != "global" || len(p.Batches) != 3 {
		t.Fatalf("explicit opt-in/scope lost: %+v", p)
	}
	for i, source := range want {
		if p.Batches[i].Source != source {
			t.Fatalf("selection order replaced scan ordering: %+v", p)
		}
	}
	for _, selected := range [][]model.SkillRef{
		{{Source: "outside/repo", Name: "not-in-scan"}},
		{{Source: first.Source + "\n", Name: first.Name}},
		{{Source: " " + first.Source, Name: first.Name}},
		{{Source: "--global", Name: first.Name}},
	} {
		p, err := (Service{}).BuildPlan(t.TempDir(), r, selected, plan.Options{})
		if err == nil || !reflect.DeepEqual(p, plan.Plan{}) {
			t.Fatalf("unknown/hostile selection produced a plan: %+v %v", p, err)
		}
	}
	// A catalog cannot launder controls through model.SkillRef.Key's trimming.
	r.Suggestions[0].Skill.Source += "\n"
	p, err = (Service{}).BuildPlan(t.TempDir(), r, nil, plan.Options{})
	if err == nil || !reflect.DeepEqual(p, plan.Plan{}) {
		t.Fatalf("hostile catalog source produced instructions: %+v %v", p, err)
	}
}

func TestBuildPlanRejectsSameSourceDestinationCollision(t *testing.T) {
	r := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
		{Skill: model.SkillRef{Source: "same/repo", Name: "Go Tools"}, Bucket: "suggested"},
		{Skill: model.SkillRef{Source: "same/repo", Name: "go-tools"}, Bucket: "suggested"},
	}}}
	if _, err := (Service{}).BuildPlan(t.TempDir(), r, nil, plan.Options{}); err == nil {
		t.Fatal("application accepted colliding names within one repository")
	}
}

func TestScanAndBuildPlanAreOfflineAndLeaveLegacyStateUntouched(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	for name, body := range map[string]string{
		"package.json":                     `{"dependencies":{"react":"1"}}`,
		"skills-lock.json":                 "malformed upstream lock",
		".skillscan/lock.json":             "malformed legacy state",
		".agents/skills/existing/SKILL.md": "existing installed skill",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := Service{RegistryBuilder: func() (*backend.Registry, error) {
		t.Fatal("offline scan or planning constructed a discovery registry")
		return nil, nil
	}}
	result, err := svc.Scan(context.Background(), root, ScanOptions{Online: false})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.BuildPlan(root, result, nil, plan.Options{})
	if err != nil || len(p.Batches) == 0 {
		t.Fatalf("no executable/state dependency allowed: %+v %v", p, err)
	}
	for name, want := range map[string]string{"skills-lock.json": "malformed upstream lock", ".skillscan/lock.json": "malformed legacy state", ".agents/skills/existing/SKILL.md": "existing installed skill"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatalf("scan/plan modified %s: %q %v", name, data, err)
		}
	}
}
