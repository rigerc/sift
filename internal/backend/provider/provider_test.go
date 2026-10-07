package provider

import (
	"testing"

	"go-s/internal/backend"
	"go-s/internal/model"
)

func TestSearchQueryUsesPrimaryAndStrongestContext(t *testing.T) {
	q := backend.Query{
		Unresolved: []model.Observation{{Key: "pkg:npm:react", Value: "react"}},
		Context: []model.MergedSignal{
			{Key: "node", Confidence: 0.4},
			{Key: "typescript", Confidence: 0.9},
		},
	}
	if got := SearchQuery(q); got != "react typescript" {
		t.Fatalf("SearchQuery = %q", got)
	}
	if got := SearchQuery(backend.Query{}); got != "" {
		t.Fatalf("empty SearchQuery = %q", got)
	}
}

func TestLimitClampsToProviderWindow(t *testing.T) {
	cases := map[int]int{0: DefaultLimit, -5: DefaultLimit, 10: 10, 999: MaxLimit}
	for input, want := range cases {
		if got := Limit(input); got != want {
			t.Fatalf("Limit(%d) = %d, want %d", input, got, want)
		}
	}
}

func TestNameIsFlagAndPathSafe(t *testing.T) {
	cases := map[string]string{
		"acme/cool-skill": "cool-skill",
		"ns:cool":         "cool",
		"-flag":           "",
		"":                "",
		"  spaced  ":      "spaced",
	}
	for input, want := range cases {
		if got := Name(input); got != want {
			t.Fatalf("Name(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"acme/repo":                    "acme/repo",
		"https://github.com/Acme/Repo": "acme/repo",
		"https://github.com/Acme/Repo/tree/main/skill": "acme/repo",
		"https://skillsmp.com/skills/x":                "",
		"":                                             "",
	}
	for input, want := range cases {
		if got := RepoFromURL(input); got != want {
			t.Fatalf("RepoFromURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSlugAndClamp(t *testing.T) {
	if got := Slug("Cool Skill!"); got != "cool-skill" {
		t.Fatalf("Slug = %q", got)
	}
	if got := Clamp01(1.5); got != 1 {
		t.Fatalf("Clamp01(1.5) = %v", got)
	}
	if got := Clamp01(-1); got != 0 {
		t.Fatalf("Clamp01(-1) = %v", got)
	}
	if got := RankScore(0); got != 1 {
		t.Fatalf("RankScore(0) = %v", got)
	}
}
