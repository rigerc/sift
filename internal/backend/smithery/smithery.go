// Package smithery adapts the @smithery/cli (4.11.1) to the unified Searcher
// contract. No error schema is documented for the CLI, so non-JSON output and
// failures surface as unknown with raw output attached for triage.
package smithery

import (
	"context"
	"encoding/json"
	"go-s/internal/backend"
	"go-s/internal/backend/clix"
	"go-s/internal/model"
	"strconv"
)

const (
	pinnedVersion = "4.11.1"
	binaryName    = "smithery"
)

var allowedFlags = []string{"--json", "--limit", "--page", "--namespace"}

type Adapter struct {
	name   string
	binary string
	runner clix.Runner
}

// New builds the adapter. cfg.URL may override the binary path for testing;
// cfg.Auth (env:NAME or literal) is treated as a secret to redact.
func New(cfg backend.Config) (backend.Backend, error) {
	binary := binaryName
	if cfg.URL != "" {
		binary = cfg.URL
	}
	inner := &clix.Exec{Secrets: clix.ResolveSecrets(cfg.Auth)}
	return &Adapter{name: cfg.Name, binary: binary, runner: clix.NewPinned(inner, binary, pinnedVersion)}, nil
}

func (a *Adapter) Name() string { return a.name }

// Probe reports the installed CLI version, failing on version drift.
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	stdout, _, err := a.runner.Run(ctx, a.binary, []string{"--version"}, []string{"--version"})
	if err != nil {
		return "", err
	}
	return clix.ParseVersion(string(stdout)), nil
}

type envelope struct {
	Results []result `json:"results"`
}
type result struct {
	QualifiedName string  `json:"qualifiedName"`
	Name          string  `json:"name"`
	Namespace     string  `json:"namespace"`
	Description   string  `json:"description"`
	Score         float64 `json:"score"`
}

func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := clix.SanitizeQuery(clix.QueryTerms(q))
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	args := []string{"skill", "search", query, "--json"}
	if q.Limit > 0 {
		args = append(args, "--limit", strconv.Itoa(q.Limit))
	}
	stdout, _, err := a.runner.Run(ctx, a.binary, allowedFlags, args)
	if err != nil {
		return nil, err
	}
	return a.parse(stdout)
}

func (a *Adapter) parse(stdout []byte) ([]backend.ExternalSuggestion, error) {
	var payload envelope
	if err := json.Unmarshal(stdout, &payload); err != nil {
		return nil, &clix.Error{Code: clix.CodeUnknown, Message: "smithery returned non-JSON output", Raw: string(stdout)}
	}
	out := make([]backend.ExternalSuggestion, 0, len(payload.Results))
	for _, r := range payload.Results {
		name := r.QualifiedName
		if name == "" {
			name = r.Name
		}
		if name == "" {
			continue
		}
		out = append(out, backend.ExternalSuggestion{
			Skill:         model.SkillRef{Source: a.name, Name: name},
			Title:         r.Description,
			SourceBackend: a.name,
			ExternalScore: r.Score,
		})
	}
	return out, nil
}
