package backend

import (
	"context"
	"go-s/internal/model"
	"testing"
)

type fakeBackend struct {
	name    string
	results []ExternalSuggestion
}
type identityBackend struct{ name string }

func (f identityBackend) Name() string { return f.name }

func (f fakeBackend) Name() string { return f.name }
func (f fakeBackend) Search(context.Context, Query) ([]ExternalSuggestion, error) {
	return f.results, nil
}

func TestRegistryPriorityAndCapabilityMerge(t *testing.T) {
	Register("test-registry", func(c Config) (Backend, error) {
		return fakeBackend{name: c.Name, results: []ExternalSuggestion{{Skill: model.SkillRef{Source: "owner/repo", Name: "a"}, ExternalScore: 0.5}, {Skill: model.SkillRef{Source: "owner/repo", Name: "a"}, ExternalScore: 0.9}}}, nil
	})
	r, err := NewRegistry([]Config{{Name: "one", Type: "test-registry", Capabilities: CapSearch}})
	if err != nil {
		t.Fatal(err)
	}
	got, errs := r.Search(context.Background(), Query{})
	if len(errs) != 0 || len(got) != 1 || got[0].ExternalScore != 0.5 {
		t.Fatalf("search = %#v, errors=%v", got, errs)
	}
}

func TestRegistryRejectsUnsupportedConfiguredCapability(t *testing.T) {
	Register("test-identity", func(c Config) (Backend, error) { return identityBackend{name: c.Name}, nil })
	if _, err := NewRegistry([]Config{{Name: "bad", Type: "test-identity", Capabilities: CapSearch}}); err == nil {
		t.Fatal("expected unsupported capability error")
	}
}
func TestRegistryAppliesGlobalLimitAcrossBackends(t *testing.T) {
	Register("test-limit", func(c Config) (Backend, error) {
		results := []ExternalSuggestion{
			{Skill: model.SkillRef{Source: c.Name, Name: "a"}, ExternalScore: 0.9},
			{Skill: model.SkillRef{Source: c.Name, Name: "b"}, ExternalScore: 0.8},
		}
		return fakeBackend{name: c.Name, results: results}, nil
	})
	r, err := NewRegistry([]Config{
		{Name: "one", Type: "test-limit", Capabilities: CapSearch},
		{Name: "two", Type: "test-limit", Capabilities: CapSearch},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, errs := r.Search(context.Background(), Query{Limit: 2})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("global limit not enforced: got %d results: %#v", len(got), got)
	}
}

func TestRegistryCalibratesBackendScoresWithResultRank(t *testing.T) {
	Register("test-ranked", func(c Config) (Backend, error) {
		var results []ExternalSuggestion
		if c.Name == "one" {
			results = []ExternalSuggestion{
				{Skill: model.SkillRef{Source: "one", Name: "first"}, ExternalScore: 0.55},
				{Skill: model.SkillRef{Source: "one", Name: "second"}, ExternalScore: 0.99},
			}
		} else {
			results = []ExternalSuggestion{
				{Skill: model.SkillRef{Source: "two", Name: "first"}, ExternalScore: 0.60},
			}
		}
		return fakeBackend{name: c.Name, results: results}, nil
	})
	r, err := NewRegistry([]Config{
		{Name: "one", Type: "test-ranked", Capabilities: CapSearch},
		{Name: "two", Type: "test-ranked", Capabilities: CapSearch},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, errs := r.Search(context.Background(), Query{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results: %#v", len(got), got)
	}
	if got[0].Skill.Name == "second" {
		t.Fatalf("raw backend score incorrectly dominated cross-backend rank: %#v", got)
	}
}
