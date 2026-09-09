package install

import (
	"context"
	"go-s/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	for _, r := range []model.SkillRef{{Source: "--evil", Name: "good"}, {Source: "a/b", Name: "../evil"}, {Source: "a/b", Name: "*"}, {Source: "https://user:secret@example.org/a", Name: "good"}, {Source: "./local", Name: "good"}, {Source: "a/b\n", Name: "good"}} {
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
}

type fakeRunner struct {
	calls int
	args  []string
	fail  bool
}

func (f *fakeRunner) Run(_ context.Context, root, _ string, args []string, _ io.Writer) error {
	f.calls++
	f.args = args
	if f.fail {
		return io.ErrUnexpectedEOF
	}
	if args[2] == "add" {
		for _, name := range args[5:] {
			if name == "--agent" {
				break
			}
			dir := filepath.Join(root, ".agents", "skills", name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
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

func TestPreflightShim(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, version := range []string{"v22.19.0", "v22.20.0", "v24.0.0"} {
		for name, body := range map[string]string{"node": "#!/bin/sh\nprintf '" + version + "\\n'\n", "npx": "#!/bin/sh\nexit 0\n"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		err := Preflight(context.Background())
		if strings.Contains(version, "19") {
			if err == nil {
				t.Fatal("old node accepted")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
