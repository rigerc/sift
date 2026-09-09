package clix

import (
	"go-s/internal/backend"
	"go-s/internal/model"
	"testing"
)

func queryFixture() backend.Query {
	return backend.Query{
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

func TestResolveSecretsFromEnv(t *testing.T) {
	t.Setenv("CLIX_TEST_TOKEN", "s3cret")
	if got := ResolveSecrets("env:CLIX_TEST_TOKEN"); len(got) != 1 || got[0] != "s3cret" {
		t.Fatalf("unexpected secrets %v", got)
	}
	if got := ResolveSecrets("env:CLIX_TEST_MISSING"); got != nil {
		t.Fatalf("expected nil for missing env, got %v", got)
	}
	if got := ResolveSecrets("literal-token"); len(got) != 1 || got[0] != "literal-token" {
		t.Fatalf("unexpected literal secrets %v", got)
	}
	if got := ResolveSecrets(""); got != nil {
		t.Fatalf("expected nil for empty auth, got %v", got)
	}
}
