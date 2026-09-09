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
	for _, r := range []model.SkillRef{{Source: "askill", Name: "docker-compose"}, {Source: "skillfish", Name: "k8s-debug"}, {Source: "smithery", Name: "@acme/mcp-fetch"}} {
		if err := ValidateRef(r, false); err != nil {
			t.Errorf("rejected backend ref %+v: %v", r, err)
		}
	}
}

func TestBuildBackendBatchesAndArgv(t *testing.T) {
	refs := []model.SkillRef{
		{Source: "a/repo", Name: "one"},
		{Source: "askill", Name: "docker-compose"},
		{Source: "askill", Name: "second-skill"},
		{Source: "smithery", Name: "@acme/mcp-fetch"},
		{Source: "skillfish", Name: "k8s-debug"},
	}
	p, err := Build(t.TempDir(), refs, Options{Agents: []string{"claude-code", "opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batches) != 5 {
		t.Fatalf("backend skills must not share batches: %+v", p.Batches)
	}
	if p.Batches[0].Backend != "" || p.Batches[0].Skills[0] != "one" {
		t.Fatalf("default batch broken: %+v", p.Batches[0])
	}
	tests := []struct {
		i       int
		backend string
		argv    []string
	}{
		{i: 1, backend: BackendAskill, argv: []string{"add", "docker-compose", "-a", "claude-code", "-a", "opencode", "-y"}},
		{i: 2, backend: BackendAskill, argv: []string{"add", "second-skill", "-a", "claude-code", "-a", "opencode", "-y"}},
		{i: 3, backend: BackendSmithery, argv: []string{"skill", "add", "@acme/mcp-fetch", "--agent", "claude-code", "--agent", "opencode"}},
		{i: 4, backend: BackendSkillfish, argv: []string{"add", "k8s-debug", "--agent", "claude-code", "--agent", "opencode", "--project", "-y"}},
	}
	for _, tt := range tests {
		b := p.Batches[tt.i]
		if b.Backend != tt.backend {
			t.Errorf("batch %d backend=%q want %q", tt.i, b.Backend, tt.backend)
		}
		if !reflect.DeepEqual(b.Argv, tt.argv) {
			t.Errorf("batch %d argv=%q want %q", tt.i, b.Argv, tt.argv)
		}
	}
	global, err := Build(t.TempDir(), refs[:2], Options{Agents: []string{"claude-code"}, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"add", "docker-compose", "-a", "claude-code", "-g", "-y"}; !reflect.DeepEqual(global.Batches[1].Argv, want) {
		t.Fatalf("global askill argv=%q", global.Batches[1].Argv)
	}
	if want := []string{"--yes", Package, "add", "a/repo", "--skill", "one", "--agent", "claude-code", "--yes", "--global"}; !reflect.DeepEqual(global.Batches[0].Argv, want) {
		t.Fatalf("global default argv=%q", global.Batches[0].Argv)
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
	if name == "npx" {
		for _, name := range args[5:] {
			if name == "--agent" {
				break
			}
			f.makeSkillDir(root, name)
		}
		return nil
	}
	// Backend CLIs: "add <skill> ..." or "skill add <skill> ...".
	if len(args) >= 2 && args[0] == "add" {
		f.makeSkillDir(root, args[1])
	}
	if len(args) >= 3 && args[0] == "skill" && args[1] == "add" {
		f.makeSkillDir(root, args[2])
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

func TestExecutionDispatchesBackendCLI(t *testing.T) {
	root := t.TempDir()
	p, err := Build(root, []model.SkillRef{
		{Source: "a/repo", Name: "one"},
		{Source: "askill", Name: "docker-compose"},
		{Source: "smithery", Name: "@acme/mcp-fetch"},
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
	if r.calls != 3 {
		t.Fatalf("calls %d", r.calls)
	}
	// Execution order: default batch first, then backend batches in plan order.
	if r.name != "smithery" {
		t.Fatalf("last invocation binary %q", r.name)
	}
	// State tracks backend sources and hashed destinations.
	statuses, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]Status{}
	for _, s := range statuses {
		bySource[s.Source] = s
	}
	if s, ok := bySource["askill"]; !ok || s.Status != "installed" {
		t.Fatalf("askill entry: %+v (have %v)", s, statuses)
	}
	if s, ok := bySource["smithery"]; !ok || s.Name != "@acme/mcp-fetch" {
		t.Fatalf("smithery entry: %+v", s)
	}
}

func TestPreflightBackendsRequiresCLIOnPath(t *testing.T) {
	dir := t.TempDir()
	p, err := Build(t.TempDir(), []model.SkillRef{{Source: "askill", Name: "x"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := PreflightBackends(p); err == nil {
		t.Fatal("missing askill binary accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "askill"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := PreflightBackends(p); err != nil {
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
