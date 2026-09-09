package askill

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
	return &Adapter{name: "askill", binary: Binary, runner: runner}
}

func TestSearchMapsDocumentedEnvelope(t *testing.T) {
	runner := &fakeRunner{stdout: `{"ok":true,"data":{"results":[
		{"slug":"docker-compose","title":"Compose helper","score":0.9},
		{"name":"golang-vet","title":"Vet wrapper","score":0.5}]}}`}
	suggestions, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "docker"}},
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("want 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Skill != (model.SkillRef{Source: "askill", Name: "docker-compose"}) {
		t.Fatalf("unexpected first suggestion %+v", suggestions[0].Skill)
	}
	if suggestions[0].ExternalScore != 0.9 || suggestions[0].SourceBackend != "askill" {
		t.Fatalf("unexpected suggestion %+v", suggestions[0])
	}
	want := []string{"find", "docker", "--json", "--limit", "5"}
	for i, arg := range want {
		if i >= len(runner.args) || runner.args[i] != arg {
			t.Fatalf("args mismatch: want %v, got %v", want, runner.args)
		}
	}
}

func TestSearchMapsDocumentedErrorCodes(t *testing.T) {
	tests := []struct {
		code string
		want clix.Code
	}{
		{code: "SKILL_NOT_FOUND", want: clix.CodeNotFound},
		{code: "MULTIPLE_SKILLS_REQUIRE_SELECTION", want: clix.CodeAmbiguous},
		{code: "INVALID_AGENTS", want: clix.CodeInvalidArgs},
		{code: "INVALID_OPTIONS", want: clix.CodeInvalidArgs},
		{code: "UNHANDLED_ERROR", want: clix.CodeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			runner := &fakeRunner{stdout: `{"ok":false,"error":{"code":"` + tt.code + `","message":"boom"}}`}
			_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
				Unresolved: []model.Observation{{Value: "x"}},
			})
			if clix.CodeOf(err) != tt.want {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
		})
	}
}

func TestSearchUnparseableOutputIsUnknown(t *testing.T) {
	runner := &fakeRunner{stdout: "not json at all"}
	_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "x"}},
	})
	if clix.CodeOf(err) != clix.CodeUnknown {
		t.Fatalf("want unknown, got %v", err)
	}
}

func TestSearchEmptyQuerySkipsExecution(t *testing.T) {
	runner := &fakeRunner{stdout: `{"ok":true,"data":{"results":[]}}`}
	out, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "-"}},
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

func TestSearchPropagatesNormalizedRunnerError(t *testing.T) {
	runner := &fakeRunner{err: &clix.Error{Code: clix.CodeNetwork, Message: "timed out"}}
	_, err := newTestAdapter(runner).Search(context.Background(), backend.Query{
		Unresolved: []model.Observation{{Value: "x"}},
	})
	if clix.CodeOf(err) != clix.CodeNetwork {
		t.Fatalf("want network code, got %v", err)
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
