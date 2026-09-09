package skillfish

import (
	"context"
	"go-s/internal/backend"
	"go-s/internal/backend/clix"
	"go-s/internal/model"
	"os"
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
	return &Adapter{name: "skillfish", binary: binaryName, runner: runner}
}

func TestSearchParsesGoldenEnvelope(t *testing.T) {
	data, err := os.ReadFile("testdata/search.golden.json")
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	runner := &fakeRunner{stdout: string(data)}
	suggestions, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "kubernetes"}},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("want 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Skill.Name != "k8s-debug" || suggestions[0].Skill.Source != "acme/skills" {
		t.Fatalf("unexpected suggestion %+v", suggestions[0].Skill)
	}
	if suggestions[1].Skill.Source != "skillfish" {
		t.Fatalf("repo-less result should fall back to backend name, got %+v", suggestions[1].Skill)
	}
	want := []string{"search", "kubernetes", "-l", "10"}
	for i, arg := range want {
		if i >= len(runner.args) || runner.args[i] != arg {
			t.Fatalf("args mismatch: want %v, got %v", want, runner.args)
		}
	}
}

func TestSearchMapsFirstErrorEntry(t *testing.T) {
	tests := []struct {
		code string
		want clix.Code
	}{
		{code: "SKILL_NOT_FOUND", want: clix.CodeNotFound},
		{code: "AUTH_REQUIRED", want: clix.CodeAuthRequired},
		{code: "NETWORK_TIMEOUT", want: clix.CodeNetwork},
		{code: "INVALID_QUERY", want: clix.CodeInvalidArgs},
		{code: "SOMETHING_ELSE", want: clix.CodeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			runner := &fakeRunner{stdout: `{"success":false,"exit_code":1,"errors":[{"code":"` + tt.code + `","message":"detail"}]}`}
			_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
				Unresolved: []model.Observation{{Value: "x"}},
			})
			if clix.CodeOf(err) != tt.want {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
		})
	}
}

func TestSearchFallsBackToExitCodeSignal(t *testing.T) {
	runner := &fakeRunner{stdout: `{"success":false,"exit_code":7,"errors":[]}`}
	_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "x"}},
	})
	var cliErr *clix.Error
	if !asClixError(err, &cliErr) {
		t.Fatalf("want *clix.Error, got %v", err)
	}
	if cliErr.Code != clix.CodeUnknown || cliErr.ExitCode != 7 {
		t.Fatalf("unexpected error %+v", cliErr)
	}
}

func TestSearchUnparseableOutputIsUnknown(t *testing.T) {
	runner := &fakeRunner{stdout: "panic: boom"}
	_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "x"}},
	})
	if clix.CodeOf(err) != clix.CodeUnknown {
		t.Fatalf("want unknown, got %v", err)
	}
}

func asClixError(err error, target **clix.Error) bool {
	e, ok := err.(*clix.Error)
	if ok {
		*target = e
	}
	return ok
}

func TestProbeReportsPinnedVersion(t *testing.T) {
	runner := &fakeRunner{stdout: "cli 9.9.9\n"}
	a := newTestAdapter(clix.NewPinned(runner, binaryName, pinnedVersion))
	if _, err := a.Probe(context.Background()); err == nil {
		t.Fatal("expected drift failure")
	}
	runner.stdout = "cli " + pinnedVersion + "\n"
	version, err := newTestAdapter(clix.NewPinned(runner, binaryName, pinnedVersion)).Probe(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != pinnedVersion {
		t.Fatalf("unexpected version %q", version)
	}
}
