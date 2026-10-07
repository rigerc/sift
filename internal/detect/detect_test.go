package detect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/walk"
)

func TestRunRetainsUnknownPackagesAndMatchesWaves(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"dependencies":{"react":"1","mystery-framework":"1"}}`)
	write("next.config.js", "module.exports={}")
	write("README.md", "docs")
	write("x.kt", "class X")
	write("build.gradle", "plugins { id 'java' }")
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rules := []model.DetectionRule{{TechnologyID: "node:react", Detect: model.DetectConfig{Packages: []string{"react"}}}, {TechnologyID: "node:next", Detect: model.DetectConfig{ConfigFiles: []model.FileRule{{Pattern: "next.config.js", Mode: model.MatchExact}}, Content: []model.ContentRule{{FilePattern: "build.gradle", Patterns: []string{"java"}}}}}, {TechnologyID: "kotlin", Detect: model.DetectConfig{FileExtensions: []model.ExtensionRule{{Extension: ".kt", MinCount: 1}}}}}
	d, err := Run(context.Background(), w, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Observations) == 0 || len(d.Signals) < 3 {
		t.Fatalf("observations=%+v signals=%+v", d.Observations, d.Signals)
	}
	found := false
	for _, o := range d.Unresolved {
		if o.Value == "mystery-framework" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown package was discarded: %+v", d.Unresolved)
	}
	for i := 1; i < len(d.Signals); i++ {
		if signalLess(d.Signals[i], d.Signals[i-1]) {
			t.Fatalf("signals not deterministic: %+v", d.Signals)
		}
	}
}

func TestRunContentCapAndGlob(t *testing.T) {
	root := t.TempDir()
	b := make([]byte, 10000)
	copy(b, []byte("needle"))
	if err := os.WriteFile(filepath.Join(root, "tool.cfg"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.cfg"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, _ := walk.Run(context.Background(), root, walk.Options{})
	rules := []model.DetectionRule{{TechnologyID: "tool", Detect: model.DetectConfig{ConfigFiles: []model.FileRule{{Pattern: "tool.*", Mode: model.MatchGlob}}, Content: []model.ContentRule{{FilePattern: "tool.cfg", Patterns: []string{"needle"}, ReadLimit: 3}}}}}
	d, err := Run(context.Background(), w, rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range d.Signals {
		if s.Layer == 4 {
			t.Fatal("content exceeded cap")
		}
	}
}

func TestRunDoesNotFollowSymlinkOutsideWorkspace(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.cfg"), []byte("secret-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.cfg"), filepath.Join(root, "app.config")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rules := []model.DetectionRule{{TechnologyID: "secret", Detect: model.DetectConfig{Content: []model.ContentRule{{FilePattern: "app.config", Patterns: []string{"secret-token"}}}}}}
	d, err := Run(context.Background(), w, rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range d.Signals {
		if s.Key == "secret" {
			t.Fatal("read content through outside symlink")
		}
	}
}

func TestManifestDependencyTables(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"pyproject.toml": "[project]\ndependencies = [\"requests>=2\"]\n[tool.poetry.dependencies]\nflask = \"*\"\n",
		"Cargo.toml":     "[dependencies]\nserde = \"1\"\n[dev-dependencies]\nproptest = \"1\"\n[target.'cfg(unix)'.dependencies]\nlibc = \"0.2\"\n",
		"composer.json":  `{"require":{"phpunit/phpunit":"*"},"require-dev":{"psalm/phar":"*"}}`,
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(root, n), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Run(context.Background(), w, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, o := range d.Observations {
		got[o.Value] = true
	}
	for _, want := range []string{"requests", "flask", "serde", "proptest", "libc", "phpunit/phpunit", "psalm/phar"} {
		if !got[want] {
			t.Errorf("missing %q in observations", want)
		}
	}
}

func TestManifestSignalFiresForGoModule(t *testing.T) {
	root := t.TempDir()
	body := "module x\n\ngo 1.25\n\nrequire (\n\tgithub.com/spf13/cobra v1.8.0 // comment\n)\n\nrequire github.com/foo/bar v0.1.0\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rules := []model.DetectionRule{{TechnologyID: "go:module", Detect: model.DetectConfig{Manifests: []string{"go.mod"}}}}
	d, err := Run(context.Background(), w, rules)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, s := range d.Signals {
		found[s.Key] = true
	}
	if !found["go:module"] {
		t.Fatalf("go:module signal missing: %+v", d.Signals)
	}
	got := map[string]bool{}
	for _, o := range d.Observations {
		if o.Kind == model.ObsPackage {
			got[o.Value] = true
		}
	}
	for _, want := range []string{"github.com/spf13/cobra", "github.com/foo/bar"} {
		if !got[want] {
			t.Errorf("go.mod parser missing %q: %v", want, got)
		}
	}
	if got["("] || got["require"] {
		t.Errorf("go.mod parser leaked syntax tokens: %v", got)
	}
}

func TestManifestObservationAbsentWithoutRule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Run(context.Background(), w, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range d.Observations {
		if o.Kind == model.ObsManifest {
			t.Fatalf("unexpected manifest observation without rules: %+v", o)
		}
	}
}
