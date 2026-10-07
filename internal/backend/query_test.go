package backend

import (
	"testing"

	"github.com/rigerc/sift/internal/model"
)

func queryFixture() Query {
	return Query{
		Unresolved: []model.Observation{{Value: "obs"}},
		Context:    []model.MergedSignal{{Key: "ctx"}},
	}
}

func TestSanitizeQueryStripsFlagInjection(t *testing.T) {
	tests := []struct {
		name  string
		terms []string
		want  string
	}{
		{name: "plain terms", terms: []string{"docker", "compose"}, want: "docker compose"},
		{name: "leading dashes stripped", terms: []string{"--rm", "-rf", "skill"}, want: "rm rf skill"},
		{name: "empty terms dropped", terms: []string{"", "  ", "go"}, want: "go"},
		{name: "all empty", terms: []string{"", "-"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeQuery(tt.terms); got != tt.want {
				t.Fatalf("SanitizeQuery(%v) = %q, want %q", tt.terms, got, tt.want)
			}
		})
	}
}

func TestQueryTermsOrder(t *testing.T) {
	q := queryFixture()
	got := QueryTerms(q)
	if len(got) != 2 || got[0] != "obs" || got[1] != "ctx" {
		t.Fatalf("unexpected terms %v", got)
	}
}

func TestQueryTermsFocusesAndDeduplicatesContext(t *testing.T) {
	q := Query{
		Unresolved: []model.Observation{{Value: "React"}},
		Context: []model.MergedSignal{
			{Key: "lang:go", Confidence: 0.4},
			{Key: "node:react", Confidence: 0.9},
			{Key: "tool:docker-compose", Confidence: 0.8},
			{Key: "docs:seo", Confidence: 0.1},
		},
	}
	got := QueryTerms(q)
	want := []string{"React", "docker-compose", "go", "seo"}
	if len(got) != len(want) {
		t.Fatalf("QueryTerms = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("QueryTerms = %v, want %v", got, want)
		}
	}
}
