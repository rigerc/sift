package install

import (
	"context"
	"go-s/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

type fakeRunner struct {
	calls int
	name  string
	args  []string
	fail  bool
}

func (f *fakeRunner) Run(_ context.Context, root, name string, args []string, _ io.Writer) error {
	f.calls++
	f.name, f.args = name, args
	if f.fail {
		return io.ErrUnexpectedEOF
	}
	for _, name := range args[5:] {
		if name == "--agent" {
			break
		}
		f.makeSkillDir(root, name)
	}
	return nil
}

func (f *fakeRunner) makeSkillDir(root, name string) {
	dir := filepath.Join(root, ".agents", "skills", canonicalSkillName(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0o644)
}

func TestExecutionAndState(t *testing.T) {
	root := t.TempDir()
	p, err := Build(root, []model.SkillRef{{Source: "a/repo", Name: "one"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if r.calls != 0 {
		t.Fatal("planning executed")
	}
	if err := Execute(context.Background(), p, r, io.Discard); err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 {
		t.Fatalf("calls %d", r.calls)
	}
	statuses, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Status != "installed" {
		t.Fatalf("%+v", statuses)
	}
	path := filepath.Join(root, ".agents", "skills", "one", "SKILL.md")
	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = Inspect(root)
	if err != nil || statuses[0].Status != "modified" {
		t.Fatalf("%+v %v", statuses, err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills-lock.json")); !os.IsNotExist(err) {
		t.Fatal("touched upstream state")
	}
}

func TestExecutionUsesNpxForEveryBatch(t *testing.T) {
	root := t.TempDir()
	p, err := Build(root, []model.SkillRef{
		{Source: "a/repo", Name: "one"},
		{Source: "b/repo", Name: "docker-compose"},
	}, Options{Agents: []string{"claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if r.calls != 0 {
		t.Fatal("planning executed")
	}
	if err := Execute(context.Background(), p, r, io.Discard); err != nil {
		t.Fatal(err)
	}
	if r.calls != 2 {
		t.Fatalf("calls %d", r.calls)
	}
	if r.name != "npx" {
		t.Fatalf("last invocation binary %q", r.name)
	}
	statuses, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]Status{}
	for _, s := range statuses {
		bySource[s.Source] = s
	}
	if s, ok := bySource["a/repo"]; !ok || s.Status != "installed" {
		t.Fatalf("a/repo entry: %+v (have %v)", s, statuses)
	}
	if s, ok := bySource["b/repo"]; !ok || s.Name != "docker-compose" {
		t.Fatalf("b/repo entry: %+v", s)
	}
}

func TestPreflightRequiresNpxOnPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := Preflight(context.Background()); err == nil {
		t.Fatal("missing npx accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "npx"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHashStableAndPathSensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := HashDirectory(dir)
	b, _ := HashDirectory(dir)
	if a != b || a == "" {
		t.Fatal("unstable hash")
	}
	if err := os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	b, _ = HashDirectory(dir)
	if a == b {
		t.Fatal("hash omitted relative paths")
	}
}
