package walk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunIndexesAndPartitionsWorkspace(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"workspaces":["packages/*"],"dependencies":{"root": "1"}}`)
	write("packages/a/package.json", `{"dependencies":{"a":"1"}}`)
	write("packages/a/main.go", "package a")
	write("packages/b/main.ts", "export {}")
	write("node_modules/ignored.js", "ignored")
	write(".gitignore", "ignored.txt\n")
	write("ignored.txt", "ignored")
	r, err := Run(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 5 {
		t.Fatalf("files=%d, want 4", len(r.Files))
	}
	if r.ExtCounts[".go"] != 1 || r.ExtCounts[".ts"] != 1 {
		t.Fatalf("extensions=%v", r.ExtCounts)
	}
	if len(r.NameIndex["package.json"]) != 2 {
		t.Fatalf("names=%v", r.NameIndex)
	}
	if len(r.Members) != 3 || r.Members[1].Root != "packages/a" || r.Members[2].Root != "packages/b" {
		t.Fatalf("members=%+v", r.Members)
	}
}

func TestRunHonorsDepthSkipAndCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "one/b.txt", "one/two/c.txt", "vendor/x.txt"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Run(context.Background(), root, Options{MaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 2 {
		t.Fatalf("files=%v", r.Files)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, root, Options{}); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestRunGoldenManifestIndexesAndMembers(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"go.work": "go 1.22\nuse (\n ./apps/web\n ./libs/core\n)\n", "apps/web/main.go": "package web\n", "libs/core/core.go": "package core\n", "README.md": "# fixture\n"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Run(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(struct {
		Files   []File
		Ext     map[string]int
		Names   map[string][]string
		Members []Member
	}{r.Files, r.ExtCounts, r.NameIndex, r.Members})
	got := string(b)
	want := `{"Files":[{"Path":"README.md","Ext":".md","Size":10},{"Path":"apps/web/main.go","Ext":".go","Size":12},{"Path":"go.work","Ext":".work","Size":41},{"Path":"libs/core/core.go","Ext":".go","Size":13}],"Ext":{".go":2,".md":1,".work":1},"Names":{"README.md":["README.md"],"core.go":["libs/core/core.go"],"go.work":["go.work"],"main.go":["apps/web/main.go"]},"Members":[{"Root":".","Files":[{"Path":"README.md","Ext":".md","Size":10},{"Path":"go.work","Ext":".work","Size":41}]},{"Root":"apps/web","Files":[{"Path":"apps/web/main.go","Ext":".go","Size":12}]},{"Root":"libs/core","Files":[{"Path":"libs/core/core.go","Ext":".go","Size":13}]}]}`
	if got != want {
		t.Fatalf("golden mismatch\n got %s\nwant %s", got, want)
	}
}

func TestRunNestedIgnoreNegation(t *testing.T) {
	root := t.TempDir()
	for n, b := range map[string]string{".gitignore": "*.tmp\n!keep.tmp\n", "keep.tmp": "ok", "sub/.gitignore": "*.log\n!keep.log\n", "sub/drop.log": "x", "sub/keep.log": "ok", "sub/a.tmp": "x"} {
		p := filepath.Join(root, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Run(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range r.Files {
		paths[f.Path] = true
	}
	for _, p := range []string{"keep.tmp", "sub/keep.log"} {
		if !paths[p] {
			t.Errorf("expected %s", p)
		}
	}
	for _, p := range []string{"sub/drop.log", "sub/a.tmp"} {
		if paths[p] {
			t.Errorf("ignored file included: %s", p)
		}
	}
}
