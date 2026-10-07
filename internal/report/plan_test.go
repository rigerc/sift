package report

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"go-s/internal/model"
	"go-s/internal/plan"
)

// Contract: both human planning flows share safe, copyable commands and a
// read-only notice. Fault: unquoted metacharacters become shell instructions.
func TestPlanText(t *testing.T) {
	t.Parallel()
	p, err := plan.Build("/tmp/work space", []model.SkillRef{{Source: "./skill source", Name: "name's $value"}}, plan.Options{Agents: []string{"codex"}, Global: true, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PlanText(&out, p); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Plan only—nothing installed", "Scope: global", "Agents: codex", "Skills: 1", "cd '/tmp/work space' && npx --yes skills add './skill source' --skill 'name'\\''s $value' --agent codex --yes --global"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}

func TestPlanTextEmpty(t *testing.T) {
	t.Parallel()
	p, err := plan.Build(t.TempDir(), nil, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PlanText(&out, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Skills: 0") || strings.Contains(out.String(), "npx ") {
		t.Fatalf("empty plan = %s", out.String())
	}
}

// Contract: commands must not inject terminal controls through the workspace
// path. Fault: shell quoting alone does not neutralize terminal OSC sequences.
func TestPlanTextRejectsTerminalControlInRoot(t *testing.T) {
	t.Parallel()
	p, err := plan.Build("/tmp/bad\x1b]0;spoof\a", []model.SkillRef{{Source: "acme/skills", Name: "one"}}, plan.Options{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := PlanText(&out, p); err == nil {
		t.Fatal("unsafe workspace path was rendered")
	}
	if out.Len() != 0 {
		t.Fatalf("invalid plan emitted instructions: %q", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPlanTextWriteError(t *testing.T) {
	t.Parallel()
	if err := PlanText(failingWriter{}, plan.Plan{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("error = %v", err)
	}
}

func TestShellJoin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ arg, want string }{
		{"", "''"}, {"plain", "plain"}, {"a b", "'a b'"}, {"a'b", "'a'\\''b'"}, {"$(touch /tmp/x)", "'$(touch /tmp/x)'"}, {"semi;colon", "'semi;colon'"},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			if got := shellJoin([]string{tc.arg}); got != tc.want {
				t.Fatalf("quote = %q, want %q", got, tc.want)
			}
		})
	}
}
