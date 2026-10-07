package prompt

import (
	"errors"
	"go-s/internal/model"
	"strings"
	"testing"
)

func fixtureResult() model.ScanResult {
	return model.ScanResult{
		Root:    "/tmp/ws",
		Signals: []model.MergedSignal{{Key: "node:react", Domain: "node"}},
		ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
			{Skill: model.SkillRef{Source: "org/local", Name: "one"}, Bucket: "suggested", Confidence: 0.9},
			{Skill: model.SkillRef{Source: "org/local", Name: "two"}, Bucket: "possible", Confidence: 0.5},
			{Skill: model.SkillRef{Source: "other/repo", Name: "three"}, Bucket: "external", ExternalScore: 7.5, SourceBackend: "official-skills", URL: "https://github.com/other/repo/tree/main/skills/three"},
			{Skill: model.SkillRef{Source: "org/local", Name: "four"}, Bucket: "hidden", Confidence: 0.1},
		}},
	}
}

func TestCountSuggestions(t *testing.T) {
	t.Parallel()
	if got := CountSuggestions(fixtureResult()); got != (ScanCounts{Suggested: 1, Possible: 1, External: 1, Hidden: 1}) {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if got := CountSuggestions(model.ScanResult{}); got != (ScanCounts{}) {
		t.Fatalf("empty result must tally to zero: %+v", got)
	}
}

func TestScanSummaryOrientation(t *testing.T) {
	t.Parallel()
	out := ScanSummary(fixtureResult())
	for _, want := range []string{
		"Workspace   /tmp/ws",
		"Detected    1 technology",
		"Skills      1 recommended · 1 possible · 1 external",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "four") {
		t.Fatalf("hidden bucket must stay headless-only:\n%s", out)
	}
	if strings.Contains(out, "Warnings") {
		t.Fatalf("no warnings expected:\n%s", out)
	}
}

func TestScanSummaryNoResults(t *testing.T) {
	t.Parallel()
	if out := ScanSummary(model.ScanResult{Root: "/x"}); !strings.Contains(out, "Skills      none") {
		t.Fatalf("expected none marker:\n%s", out)
	}
}

func TestScanSummaryWarningCap(t *testing.T) {
	t.Parallel()
	res := model.ScanResult{Root: "/x", Warnings: []string{"w1", "w2", "w3", "w4", "w5"}}
	out := ScanSummary(res)
	for _, want := range []string{"Warnings: 5", "• w1", "• w2", "• w3", "• 2 more"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "w4") {
		t.Fatalf("capped warnings must not leak w4:\n%s", out)
	}
	out = ScanSummary(model.ScanResult{Root: "/x", Warnings: []string{"only one"}})
	if !strings.Contains(out, "Warnings\n• only one") || strings.Contains(out, "Warnings:") {
		t.Fatalf("singular warning format:\n%s", out)
	}
}

func TestOptionLabelLocalVsExternal(t *testing.T) {
	t.Parallel()
	suggestions := fixtureResult().Suggestions
	suggested := OptionLabel(suggestions[0], 4)
	if !strings.Contains(suggested, "RECOMMENDED  one   90% confidence") {
		t.Fatalf("suggested label: %q", suggested)
	}
	external := OptionLabel(suggestions[2], 4)
	if !strings.Contains(external, "score 7.50") || !strings.Contains(external, "official-skills · other/repo") {
		t.Fatalf("external label: %q", external)
	}
	if !strings.Contains(external, "https://github.com/other/repo/tree/main/skills/three") {
		t.Fatalf("external label must carry its GitHub URL: %q", external)
	}
	if strings.Contains(external, "confidence") {
		t.Fatalf("external score must never read as confidence: %q", external)
	}
	if got := strings.Count(external, "\n"); got != 1 {
		t.Fatalf("labels are two-line, got %d newlines: %q", got, external)
	}
}

func TestOptionLabelStaleAndRounding(t *testing.T) {
	t.Parallel()
	stale := model.Suggestion{Skill: model.SkillRef{Source: "ext/src", Name: "old"}, Bucket: "external", ExternalScore: 0.904, Stale: true}
	if label := OptionLabel(stale, 3); !strings.Contains(label, "score 0.90 · stale") {
		t.Fatalf("stale marker missing: %q", label)
	}
	// 0.949 * 100 rounds to 95; truncation would print 94.
	rounded := model.Suggestion{Skill: model.SkillRef{Source: "o/r", Name: "n"}, Bucket: "suggested", Confidence: 0.949}
	if label := OptionLabel(rounded, 1); !strings.Contains(label, "95% confidence") {
		t.Fatalf("confidence must round: %q", label)
	}
}

func TestOptionLabelFallsBackToEvidence(t *testing.T) {
	t.Parallel()
	s := model.Suggestion{Skill: model.SkillRef{Source: "o/r", Name: "n"}, Bucket: "possible", Confidence: 0.6, Evidence: []string{"package.json"}}
	if label := OptionLabel(s, 1); !strings.Contains(label, "o/r · package.json") {
		t.Fatalf("evidence fallback missing: %q", label)
	}
}

func TestOptionLabelSanitizesInput(t *testing.T) {
	t.Parallel()
	s := model.Suggestion{
		Skill:   model.SkillRef{Source: "o/r\x1b[31m", Name: "evil\tskill"},
		Bucket:  "suggested",
		Reasons: []string{"a\x07b"},
	}
	label := OptionLabel(s, 9)
	if strings.ContainsAny(label, "\x1b\x07") {
		t.Fatalf("control chars must be sanitized: %q", label)
	}
	if got := strings.Count(label, "\n"); got != 1 {
		t.Fatalf("injected newline must collapse to a space, got %d newlines: %q", got, label)
	}
	if !strings.Contains(label, "evil skill") {
		t.Fatalf("sanitized name expected: %q", label)
	}
}

func TestOptionsSkipHiddenAndPreselectSuggested(t *testing.T) {
	t.Parallel()
	opts := Options(fixtureResult())
	if len(opts) != 3 {
		t.Fatalf("expected 3 selectable options, got %d", len(opts))
	}
	for _, s := range fixtureResult().Suggestions {
		if s.Bucket == "hidden" {
			continue
		}
		spec := optionSpecFor(s, 5)
		if spec.Key != s.Skill.Key() {
			t.Fatalf("key must stay canonical: %q", spec.Key)
		}
		if spec.Checked != (s.Bucket == "suggested") {
			t.Fatalf("only suggested must be preselected: %q checked=%v", spec.Key, spec.Checked)
		}
		if !strings.Contains(spec.Label, s.Skill.Name) {
			t.Fatalf("label must show the skill: %q", spec.Label)
		}
	}
}

func TestOptionsAlignScoreColumn(t *testing.T) {
	t.Parallel()
	res := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
		{Skill: model.SkillRef{Source: "o/r", Name: "ab"}, Bucket: "suggested", Confidence: 0.9},
		{Skill: model.SkillRef{Source: "o/r", Name: "abcdef"}, Bucket: "possible", Confidence: 0.5},
	}}}
	widths := []int{}
	for _, o := range Options(res) {
		first, _, _ := strings.Cut(o.Key, "\n")
		widths = append(widths, len(first))
	}
	if widths[0] != widths[1] {
		t.Fatalf("score columns must align: %v", widths)
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
	in := []string{"other/repo\x00three", "org/local\x00one"}
	got := FilterSelected(res, in)
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "three" {
		t.Fatalf("order not restored: %#v", got)
	}
	if got := FilterSelected(res, []string{"nope\x00x", "org/local\x00one", "org/local\x00one"}); len(got) != 1 {
		t.Fatalf("unknown/dup keys must be dropped, got %#v", got)
	}
}

func TestSelectorHeight(t *testing.T) {
	t.Parallel()
	one := Options(model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
		{Skill: model.SkillRef{Source: "o/r", Name: "one"}, Bucket: "suggested", Confidence: 0.9},
	}}})
	// One two-line option: 2 content rows + title + description + one spare.
	if got := selectorHeight(one, 20); got != 5 {
		t.Fatalf("tight sizing: got %d want 5", got)
	}
	// Never exceeds the rows available after chrome; viewport scrolls instead.
	if got := selectorHeight(one, 4); got != 4 {
		t.Fatalf("clamped to available rows: got %d want 4", got)
	}
	many := Options(model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{
		{Skill: model.SkillRef{Source: "o/r", Name: "one"}, Bucket: "suggested", Confidence: 0.9},
		{Skill: model.SkillRef{Source: "o/r", Name: "two"}, Bucket: "possible", Confidence: 0.5},
		{Skill: model.SkillRef{Source: "o/r", Name: "three"}, Bucket: "external", ExternalScore: 1.5},
	}}})
	if got := selectorHeight(many, 8); got != 8 {
		t.Fatalf("three two-line options must clamp: got %d want 8", got)
	}
}

func TestNonTTYSelectSkills(t *testing.T) {
	t.Parallel()
	// This guard fails fast before touching the terminal, so it holds in
	// CI (non-TTY). On a developer TTY interactive forms cannot be asserted.
	if IsTTY() {
		t.Skip("TTY attached; interactive forms cannot be asserted here")
	}
	if _, err := SelectSkills(fixtureResult(), "default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("SelectSkills: got %v", err)
	}
}
