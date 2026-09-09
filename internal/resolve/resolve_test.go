package resolve

import (
	"go-s/internal/model"
	"go-s/internal/rules"
	"testing"
)

func TestRunEmbeddedCatalogAndUnresolvedEligibility(t *testing.T) {
	catalog, err := rules.Load("")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run([]model.MergedSignal{{Key: "node:nextjs", Domain: "node", Confidence: 0.8, Evidence: []string{"package.json"}}}, []model.Observation{
		{Kind: model.ObsPackage, Key: "pkg:npm:unknown-framework", Value: "unknown-framework"},
		{Kind: model.ObsExt, Key: "ext:.xyz", Value: ".xyz"},
	}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suggestions) != 2 {
		t.Fatalf("suggestions = %#v", result.Suggestions)
	}
	if result.Suggestions[0].Skill.Source != "vercel-labs/agent-skills" {
		t.Fatalf("unexpected source: %#v", result.Suggestions)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0].Value != "unknown-framework" {
		t.Fatalf("unresolved = %#v", result.Unresolved)
	}
}

func TestRunKeepsSameNameDifferentSourcesAndDetectsCycles(t *testing.T) {
	catalog, err := rules.Load("")
	if err != nil {
		t.Fatal(err)
	}
	catalog.SkillRules = []model.SkillRule{{TechnologyID: "node:react", Skills: []model.SkillRef{{Source: "one/repo", Name: "shared"}, {Source: "two/repo", Name: "shared"}}}}
	catalog.ComboRules = nil
	result, err := Run([]model.MergedSignal{{Key: "node:react", Confidence: 0.9}}, nil, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suggestions) != 2 || result.Suggestions[0].Skill.Key() == result.Suggestions[1].Skill.Key() {
		t.Fatalf("source collision merged: %#v", result.Suggestions)
	}
	catalog.ComboRules = []model.ComboRule{{ID: "cycle", Triggers: []string{"node:react"}, Order: []model.SkillRef{{Source: "one/repo", Name: "shared"}, {Source: "two/repo", Name: "shared"}, {Source: "one/repo", Name: "shared"}}}}
	if _, err := Run([]model.MergedSignal{{Key: "node:react", Confidence: 0.9}}, nil, catalog); err == nil {
		t.Fatal("expected ordering cycle error")
	}
}
