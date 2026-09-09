package backend

import (
	"context"
	"go-s/internal/model"
	"testing"
)

type fakeBackend struct {
	name       string
	results    []ExternalSuggestion
	validation Validation
}
type identityBackend struct{ name string }

func (f identityBackend) Name() string { return f.name }

func (f fakeBackend) Name() string { return f.name }
func (f fakeBackend) Search(context.Context, Query) ([]ExternalSuggestion, error) {
	return f.results, nil
}

func (f fakeBackend) Validate(_ context.Context, skills []model.SkillRef) (map[string]Validation, error) {
	out := map[string]Validation{}
	for _, s := range skills {
		out[s.Key()] = f.validation
	}
	return out, nil
}

func TestRegistryPriorityAndCapabilityMerge(t *testing.T) {
	Register("test-registry", func(c Config) (Backend, error) {
		return fakeBackend{name: c.Name, results: []ExternalSuggestion{{Skill: model.SkillRef{Source: "owner/repo", Name: "a"}, ExternalScore: 0.5}, {Skill: model.SkillRef{Source: "owner/repo", Name: "a"}, ExternalScore: 0.9}}}, nil
	})
	r, err := NewRegistry([]Config{{Name: "one", Type: "test-registry", Capabilities: CapSearch}}, StrategyFanout)
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
	if _, err := NewRegistry([]Config{{Name: "bad", Type: "test-identity", Capabilities: CapSearch}}, StrategyFanout); err == nil {
		t.Fatal("expected unsupported capability error")
	}
}
