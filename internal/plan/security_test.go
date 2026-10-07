package plan

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/rigerc/sift/internal/model"
)

func TestBuildRejectsRawHostileReferencesBeforeCanonicalization(t *testing.T) {
	for _, source := range []string{
		"a/repo\n", "\na/repo", "a/repo\r", "a/repo\t", "a/repo\x00", "a/repo\x1b",
		"https://github.com/Acme/Repo/\n", "https://github.com/Acme/Repo/\u0085",
		"https://github.com/-evil/repo", "https://github.com/%2Devil/repo",
		"https://example.org/a%0a", "https://github.com/acme/repo%0a",
		" a/repo", "a/repo ", "--global", "http://example.org/skill",
		"https://user:secret@example.org/skill", "https://example.org/skill?arg=x",
		"https://example.org/skill#fragment", "https://example.org", ".owner/repo", "owner/.repo",
		strings.Repeat("x", 2049),
	} {
		t.Run(strconv.Quote(source), func(t *testing.T) {
			p, err := Build(t.TempDir(), []model.SkillRef{{Source: "safe/repo", Name: "safe"}, {Source: source, Name: "other"}}, Options{})
			if err == nil {
				t.Fatalf("accepted raw source %q: %+v", source, p)
			}
			if !reflect.DeepEqual(p, Plan{}) {
				t.Fatalf("returned partial executable instructions on error: %+v", p)
			}
		})
	}
	for _, name := range []string{"", "-flag", "--agent", ".", "..", "*", "../escape", `with\slash`, "skill\n", "skill\x1b", "...", "!!!", strings.Repeat("x", 201)} {
		if _, err := Build(t.TempDir(), []model.SkillRef{{Source: "acme/repo", Name: name}}, Options{}); err == nil {
			t.Errorf("accepted hostile/empty destination name %q", name)
		}
	}
}

func TestBuildRejectsDistinctNamesSharingDestinationEvenWithinSource(t *testing.T) {
	for _, pair := range [][2]string{{"Go Tools", "go-tools"}, {"React", "react"}, {"skill!", "skill?"}, {" skill ", "skill"}, {"foo--bar", "foo bar"}} {
		_, err := Build(t.TempDir(), []model.SkillRef{{Source: "acme/repo", Name: pair[0]}, {Source: "https://github.com/Acme/Repo/", Name: pair[1]}}, Options{})
		if err == nil || !strings.Contains(err.Error(), "destination collision") {
			t.Errorf("names %q sharing a destination were accepted: %v", pair, err)
		}
	}
}

func TestBuildDeduplicatesCanonicalIdentitiesWithoutRegrouping(t *testing.T) {
	refs := []model.SkillRef{
		{Source: "Acme/Repo/", Name: "first"},
		{Source: "acme/repo", Name: "first"},
		{Source: "https://github.com/Acme/Repo/", Name: "second"},
		{Source: "other/repo", Name: "third"},
		{Source: "acme/repo", Name: "first"},
		{Source: "acme/repo", Name: "fourth"},
	}
	agents := []string{"opencode", "codex", "codex"}
	originalRefs, originalAgents := append([]model.SkillRef(nil), refs...), append([]string(nil), agents...)
	root := t.TempDir()
	p, err := Build(root, refs, Options{Agents: agents})
	if err != nil {
		t.Fatal(err)
	}
	wantSkills := [][]string{{"first", "second"}, {"third"}, {"fourth"}}
	if len(p.Batches) != len(wantSkills) {
		t.Fatalf("batch ordering changed: %+v", p)
	}
	for i, want := range wantSkills {
		if !reflect.DeepEqual(p.Batches[i].Skills, want) {
			t.Fatalf("batch %d = %+v, want %v", i, p.Batches[i], want)
		}
	}
	if !reflect.DeepEqual(p.Agents, []string{"codex", "opencode"}) || !reflect.DeepEqual(refs, originalRefs) || !reflect.DeepEqual(agents, originalAgents) {
		t.Fatalf("mutated inputs or unstable agent ordering: %+v %v %v", p, refs, agents)
	}
	agents[0] = "changed"
	refs[0].Name = "changed"
	if p.Agents[1] != "opencode" || p.Batches[0].Skills[0] != "first" {
		t.Fatal("plan aliases caller-owned input slices")
	}
	again, err := Build(root, originalRefs, Options{Agents: originalAgents})
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatalf("non-deterministic plan: %+v %v", again, err)
	}
}

func TestBuildAgentDetectionAndExplicitAgents(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".opencode", ".codex", ".github"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A marker must be a directory, not a similarly named regular file.
	if err := os.WriteFile(filepath.Join(root, ".claude"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Build(root, nil, Options{})
	if err != nil || !reflect.DeepEqual(p.Agents, []string{"codex", "github-copilot", "opencode"}) {
		t.Fatalf("detected agents: %+v %v", p, err)
	}
	p, err = Build(root, nil, Options{Agents: []string{"claude-code"}, Global: true})
	if err != nil || p.Scope != "global" || !reflect.DeepEqual(p.Agents, []string{"claude-code"}) {
		t.Fatalf("explicit agents/scope: %+v %v", p, err)
	}
	for _, agent := range []string{"", "other", "--global", "codex\n"} {
		if _, err := Build(root, nil, Options{Agents: []string{agent}}); err == nil {
			t.Errorf("accepted invalid agent %q", agent)
		}
	}
}

func TestBuildEmptyJSONAndOptInLocalSources(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	p, err := Build(root, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(data), `"batches":[]`) || !strings.Contains(string(data), `"agents":["claude-code"]`) || p.Scope != "project" || p.Root != root {
		t.Fatalf("empty plan schema: %s %v", data, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("planning created nonexistent root: %v", err)
	}
	for _, source := range []string{"./local skill", "./LocalSkill", "../LocalSkill", filepath.Join(root, "skill")} {
		refs := []model.SkillRef{{Source: source, Name: "name with 'quotes' and $shell; text"}}
		if _, err := Build(root, refs, Options{}); err == nil {
			t.Errorf("local source %q accepted without opt-in", source)
		}
		p, err := Build(root, refs, Options{AllowLocal: true})
		if err != nil || p.Batches[0].Argv[3] != source || p.Batches[0].Argv[5] != refs[0].Name {
			t.Fatalf("local source/literal argument not preserved: %+v %v", p, err)
		}
	}
}

func TestBuildDoesNotExecuteOrReadOrWriteInstallationState(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	files := map[string]string{
		".skillscan/lock.json": "malformed legacy {", "skills-lock.json": "not json",
		".agents/skills/one/SKILL.md": "existing installed bytes",
		".claude/skills/one/SKILL.md": "agent installed bytes",
		"config.json":                 "legacy config unchanged",
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(root, "executed")
	for _, executable := range []string{"npx", "node", "narsil-mcp"} {
		if err := os.WriteFile(filepath.Join(bin, executable), []byte("#!/bin/sh\nprintf executed > '"+marker+"'\nexit 77\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot(t, root)
	for _, path := range []string{bin, t.TempDir()} {
		t.Setenv("PATH", path)
		for _, global := range []bool{false, true} {
			p, err := Build(root, []model.SkillRef{{Source: "acme/repo", Name: "one"}}, Options{Global: global})
			if err != nil || len(p.Batches) != 1 {
				t.Fatalf("state or missing executables affected planning: %+v %v", p, err)
			}
		}
	}
	if after := snapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("planning changed workspace bytes/metadata:\nbefore=%v\nafter=%v", before, after)
	}
}

type fileSnapshot struct {
	Mode     fs.FileMode
	ModTime  int64
	Contents string
}

func snapshot(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	out := map[string]fileSnapshot{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		value := fileSnapshot{Mode: info.Mode(), ModTime: info.ModTime().UnixNano()}
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value.Contents = string(data)
		}
		out[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Guard the planner's dependency boundary as well as its runtime behavior:
// installation-state and process helpers must not sneak back through imports.
func TestPlannerDependencyBoundary(t *testing.T) {
	allowed := map[string]bool{"fmt": true, "net/url": true, "os": true, "path/filepath": true, "regexp": true, "slices": true, "strings": true, "unicode": true, "github.com/rigerc/sift/internal/model": true}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if imported, ok := node.(*ast.ImportSpec); ok {
				path, err := strconv.Unquote(imported.Path.Value)
				if err != nil || !allowed[path] {
					t.Errorf("planner import escaped structural boundary: %s", imported.Path.Value)
				}
			}
			return true
		})
	}
}
