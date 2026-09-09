package prompt

import (
	"errors"
	"go-s/internal/install"
	"go-s/internal/model"
	"strings"
	"testing"
)

func fixtureResult() model.ScanResult {
	return model.ScanResult{
		Root: "/tmp/ws",
		ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
			{Skill: model.SkillRef{Source: "org/local", Name: "one"}, Bucket: "suggested", Confidence: 0.9},
			{Skill: model.SkillRef{Source: "org/local", Name: "two"}, Bucket: "possible", Confidence: 0.5},
			{Skill: model.SkillRef{Source: "ext/src", Name: "three"}, Bucket: "external", ExternalScore: 7.5},
			{Skill: model.SkillRef{Source: "org/local", Name: "four"}, Bucket: "hidden", Confidence: 0.1},
		}},
	}
}

func TestPreselectedIsSuggestedOnly(t *testing.T) {
	t.Parallel()
	got := Preselected(fixtureResult())
	if len(got) != 1 {
		t.Fatalf("expected one preselected key, got %#v", got)
	}
	want := model.SkillRef{Source: "org/local", Name: "one"}.Key()
	if got[0] != want {
		t.Fatalf("got %q want %q", got[0], want)
	}
}

func TestFilterSelectedRestoresResolverOrder(t *testing.T) {
	t.Parallel()
	res := fixtureResult()
	// Reversed user order must come back in resolver order.
	in := []string{"ext/src\x00three", "org/local\x00one"}
	got := FilterSelected(res, in)
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "three" {
		t.Fatalf("order not restored: %#v", got)
	}
	if got := FilterSelected(res, []string{"nope\x00x", "org/local\x00one", "org/local\x00one"}); len(got) != 1 {
		t.Fatalf("unknown/dup keys must be dropped, got %#v", got)
	}
}

func TestOptionsSkipsHidden(t *testing.T) {
	t.Parallel()
	for _, o := range Options(fixtureResult()) {
		if o.Value == "org/local\x00four" {
			t.Fatal("hidden suggestion must stay headless-only")
		}
	}
}

func TestPlanSummary(t *testing.T) {
	t.Parallel()
	p := install.Plan{
		Scope:  "project",
		Agents: []string{"claude-code"},
		Batches: []install.Batch{
			{Source: "org/a", Skills: []string{"one", "two"}},
			{Source: "org/b", Skills: []string{"three"}},
		},
	}
	out := PlanSummary(p)
	if !strings.Contains(out, "3 skill(s)") || !strings.Contains(out, "org/a: one, two") {
		t.Fatalf("unexpected summary: %q", out)
	}
}

func TestNonTTYPromptErrors(t *testing.T) {
	t.Parallel()
	// These guards fail fast before touching the terminal, so they hold
	// in CI (non-TTY). On a developer TTY only assert the heads-up error path.
	if IsTTY() {
		t.Skip("TTY attached; interactive forms cannot be asserted here")
	}
	if _, err := SelectSkills(fixtureResult(), "default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("SelectSkills: got %v", err)
	}
	if _, err := ConfirmPlan(install.Plan{}, "default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("ConfirmPlan: got %v", err)
	}
	if err := ShowWelcome("default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("ShowWelcome: got %v", err)
	}
	if err := RequireTTY("x"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("RequireTTY must wrap ErrNonTTY: %v", err)
	}
}
