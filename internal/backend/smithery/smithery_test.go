package smithery

import (
	"context"
	"go-s/internal/backend"
	"go-s/internal/backend/clix"
	"go-s/internal/model"
	"testing"
)

type fakeRunner struct {
	binary string
	args   []string
	stdout string
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, binary string, allowed []string, args []string) ([]byte, []byte, error) {
	f.binary, f.args = binary, args
	if f.err != nil {
		return nil, nil, f.err
	}
	return []byte(f.stdout), nil, nil
}

func newTestAdapter(runner clix.Runner) *Adapter {
	return &Adapter{name: "smithery", binary: Binary, runner: runner}
}

func TestSearchParsesResults(t *testing.T) {
	runner := &fakeRunner{stdout: `{"results":[
		{"qualifiedName":"@acme/mcp-fetch","description":"Fetch helper","score":0.95},
		{"name":"plain-name","description":"No qualifier","score":0.3}]}`}
	suggestions, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "fetch"}},
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("want 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Skill.Name != "@acme/mcp-fetch" {
		t.Fatalf("unexpected name %q", suggestions[0].Skill.Name)
	}
	if suggestions[1].Skill.Name != "plain-name" {
		t.Fatalf("expected name fallback, got %q", suggestions[1].Skill.Name)
	}
	want := []string{"skill", "search", "fetch", "--json", "--limit", "20"}
	for i, arg := range want {
		if i >= len(runner.args) || runner.args[i] != arg {
			t.Fatalf("args mismatch: want %v, got %v", want, runner.args)
		}
	}
}

func TestSearchNonJSONOutputIsUnknownWithRaw(t *testing.T) {
	runner := &fakeRunner{stdout: "Error: not logged in"}
	_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "fetch"}},
	})
	var cliErr *clix.Error
	e, ok := err.(*clix.Error)
	if !ok {
		t.Fatalf("want *clix.Error, got %v", err)
	}
	cliErr = e
	if cliErr.Code != clix.CodeUnknown {
		t.Fatalf("want unknown, got %s", cliErr.Code)
	}
	if cliErr.Raw != "Error: not logged in" {
		t.Fatalf("raw output not preserved: %q", cliErr.Raw)
	}
}

func TestSearchEmptyQuerySkipsExecution(t *testing.T) {
	runner := &fakeRunner{stdout: `{"results":[]}`}
	out, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Context: []model.MergedSignal{{Key: "-"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("want empty result, got %+v", out)
	}
	if runner.args != nil {
		t.Fatalf("expected no execution, got %v", runner.args)
	}
}

func TestProbeReportsPinnedVersion(t *testing.T) {
	runner := &fakeRunner{stdout: "cli 9.9.9\n"}
	a := newTestAdapter(clix.NewPinned(runner, Binary, PinnedVersion))
	if _, err := a.Probe(context.Background()); err == nil {
		t.Fatal("expected drift failure")
	}
	runner.stdout = "cli " + PinnedVersion + "\n"
	version, err := newTestAdapter(clix.NewPinned(runner, Binary, PinnedVersion)).Probe(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != PinnedVersion {
		t.Fatalf("unexpected version %q", version)
	}
}
