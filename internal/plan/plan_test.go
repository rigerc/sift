package plan

import (
	"reflect"
	"testing"

	"github.com/rigerc/sift/internal/model"
)

func TestOrderedBatchesAndCollision(t *testing.T) {
	refs := []model.SkillRef{{Source: "a/repo", Name: "one"}, {Source: "b/repo", Name: "two"}, {Source: "a/repo", Name: "three"}}
	p, err := Build(t.TempDir(), refs, Options{Agents: []string{"github-copilot"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 3 {
		t.Fatalf("regrouped batches: %+v", p)
	}
	want := []string{"--yes", Package, "add", "a/repo", "--skill", "one", "--agent", "github-copilot", "--yes"}
	if !reflect.DeepEqual(p.Batches[0].Argv, want) {
		t.Fatalf("argv: %q", p.Batches[0].Argv)
	}
	_, err = Build(t.TempDir(), []model.SkillRef{{Source: "a/repo", Name: "same"}, {Source: "b/repo", Name: "same"}}, Options{})
	if err == nil {
		t.Fatal("accepted destination collision")
	}
}

func TestBuildCanonicalizesSourceAndUpstreamSkillDestination(t *testing.T) {
	p, err := Build(t.TempDir(), []model.SkillRef{{Source: "https://github.com/Acme/Skills/", Name: "Go Tools"}}, Options{Agents: []string{"claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Batches[0].Source != "acme/skills" {
		t.Fatalf("source=%q", p.Batches[0].Source)
	}
	if got := canonicalSkillName(p.Batches[0].Skills[0]); got != "go-tools" {
		t.Fatalf("destination=%q", got)
	}
	if _, err := Build(t.TempDir(), []model.SkillRef{{Source: "a/repo", Name: "Go Tools"}, {Source: "b/repo", Name: "go-tools"}}, Options{Agents: []string{"claude-code"}}); err == nil {
		t.Fatal("expected sanitized destination collision")
	}
}

func TestStructuralValidation(t *testing.T) {
	for _, r := range []model.SkillRef{{Source: "--evil", Name: "good"}, {Source: "a/b", Name: "../evil"}, {Source: "a/b", Name: "*"}, {Source: "https://user:secret@example.org/a", Name: "good"}, {Source: "./local", Name: "good"}, {Source: "a/b\n", Name: "good"}, {Source: "not-a-backend", Name: "with/slash"}} {
		if ValidateRef(r, false) == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	if err := ValidateRef(model.SkillRef{Source: "a/b", Name: "Skill With Spaces"}, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRef(model.SkillRef{Source: "./local", Name: "good"}, true); err != nil {
		t.Fatal(err)
	}
	for _, r := range []model.SkillRef{{Source: "acme/skills", Name: "docker-compose"}, {Source: "https://github.com/acme/skills", Name: "plain"}} {
		if err := ValidateRef(r, false); err != nil {
			t.Errorf("rejected GitHub ref %+v: %v", r, err)
		}
	}
}

func TestBuildGroupsBatchesAndArgv(t *testing.T) {
	refs := []model.SkillRef{
		{Source: "a/repo", Name: "one"},
		{Source: "b/repo", Name: "docker-compose"},
		{Source: "b/repo", Name: "second-skill"},
		{Source: "c/repo", Name: "plain"},
	}
	p, err := Build(t.TempDir(), refs, Options{Agents: []string{"claude-code", "opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 3 {
		t.Fatalf("same-source skills must share a batch: %+v", p.Batches)
	}
	if p.Batches[0].Source != "a/repo" || p.Batches[0].Skills[0] != "one" {
		t.Fatalf("default batch broken: %+v", p.Batches[0])
	}
	if p.Batches[1].Source != "b/repo" || len(p.Batches[1].Skills) != 2 {
		t.Fatalf("grouped batch broken: %+v", p.Batches[1])
	}
	want := []string{"--yes", Package, "add", "b/repo", "--skill", "docker-compose", "second-skill", "--agent", "claude-code", "opencode", "--yes"}
	if !reflect.DeepEqual(p.Batches[1].Argv, want) {
		t.Fatalf("argv: %q", p.Batches[1].Argv)
	}
	global, err := Build(t.TempDir(), refs[:1], Options{Agents: []string{"claude-code"}, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--yes", Package, "add", "a/repo", "--skill", "one", "--agent", "claude-code", "--yes", "--global"}; !reflect.DeepEqual(global.Batches[0].Argv, want) {
		t.Fatalf("global argv=%q", global.Batches[0].Argv)
	}
}
