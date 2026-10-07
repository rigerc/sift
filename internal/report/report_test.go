package report

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigerc/sift/internal/model"
)

func TestGoldenTable(t *testing.T) {
	r := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{{Skill: model.SkillRef{Source: "owner/repo", Name: "one"}, Bucket: "suggested", Confidence: .95, Reasons: []string{"declared dependency"}, Evidence: []string{"go.mod"}}, {Skill: model.SkillRef{Source: "other/repo", Name: "one"}, Bucket: "external", ExternalScore: 42, URL: "https://github.com/other/repo/tree/main/skills/one", Reasons: []string{"backend match"}}}}}
	var out bytes.Buffer
	if err := Table(&out, r, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "table.golden.txt")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, out.Bytes()) {
		t.Fatal("table golden mismatch")
	}
}

func TestPlainRemovesControls(t *testing.T) {
	if got := Plain("bad\x1b[2J\ntext"); got != "bad [2J text" {
		t.Fatalf("%q", got)
	}
}
