package prompt

import (
	"go-s/internal/install"
	"strings"
	"testing"
)

func TestPlanSummary(t *testing.T) {
	t.Parallel()
	p := install.Plan{
		Scope:  "project",
		Agents: []string{"claude-code", "opencode"},
		Batches: []install.Batch{
			{Source: "org/a", Skills: []string{"one", "two"}},
			{Source: "ext/b", Skills: []string{"three"}},
		},
	}
	out := PlanSummary(p)
	for _, want := range []string{
		"Install 3 skills",
		"Project scope",
		"Agents: claude-code, opencode",
		"org/a\n  • one\n  • two",
		"ext/b\n  • three",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestPlanSummarySingularAndDefaults(t *testing.T) {
	t.Parallel()
	p := install.Plan{Scope: "global", Agents: []string{"claude-code"}, Batches: []install.Batch{{Source: "org/a", Skills: []string{"one"}}}}
	out := PlanSummary(p)
	if !strings.Contains(out, "Install 1 skill\n") || !strings.Contains(out, "Global scope") {
		t.Fatalf("singular/global format:\n%s", out)
	}
	if out := PlanSummary(install.Plan{}); !strings.Contains(out, "Install 0 skills\n") || !strings.Contains(out, "Project scope") {
		t.Fatalf("empty plan defaults:\n%s", out)
	}
}

func TestPlanSummarySanitizes(t *testing.T) {
	t.Parallel()
	p := install.Plan{
		Scope:   "project",
		Agents:  []string{"claude-code"},
		Batches: []install.Batch{{Source: "o/r\x1b[31m", Skills: []string{"bad\x07name"}}},
	}
	out := PlanSummary(p)
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("control chars must be sanitized: %q", out)
	}
}
