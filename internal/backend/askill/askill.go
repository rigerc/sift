// Package askill adapts the askill CLI (0.1.15) to the unified Searcher
// contract. Only commands confirmed by the CLI reference are used; unsupported
// surfaces are absent rather than assumed.
package askill

import (
	"context"
	"encoding/json"
	"go-s/internal/backend"
	"go-s/internal/backend/clix"
	"go-s/internal/model"
	"strconv"
)

const (
	pinnedVersion = "0.1.15"
	binaryName    = "askill"
)

var allowedFlags = []string{"--json", "--limit", "--page", "--tag"}

// Adapter implements backend.Searcher via the askill process exec layer.
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
	OK    bool      `json:"ok"`
	Data  results   `json:"data"`
	Error *apiError `json:"error"`
}
type apiError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}
type results struct {
	Results []result `json:"results"`
}
type result struct {
	Slug  string  `json:"slug"`
	Name  string  `json:"name"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := clix.SanitizeQuery(clix.QueryTerms(q))
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	args := []string{"find", query, "--json"}
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
		return nil, &clix.Error{Code: clix.CodeUnknown, Message: "askill returned unparseable output", Raw: string(stdout)}
	}
	if payload.Error != nil {
		return nil, &clix.Error{Code: mapAPIError(payload.Error.Code), Message: payload.Error.Message, Raw: string(stdout)}
	}
	out := make([]backend.ExternalSuggestion, 0, len(payload.Data.Results))
	for _, r := range payload.Data.Results {
		name := r.Slug
		if name == "" {
			name = r.Name
		}
		if name == "" {
			continue
		}
		out = append(out, backend.ExternalSuggestion{
			Skill:         model.SkillRef{Source: a.name, Name: name},
			Title:         r.Title,
			SourceBackend: a.name,
			ExternalScore: r.Score,
		})
	}
	return out, nil
}

// mapAPIError translates askill's documented error codes; anything else stays
// unknown so callers never branch on backend-specific strings.
func mapAPIError(code string) clix.Code {
	switch code {
	case "SKILL_NOT_FOUND":
		return clix.CodeNotFound
	case "MULTIPLE_SKILLS_REQUIRE_SELECTION":
		return clix.CodeAmbiguous
	case "INVALID_AGENTS", "INVALID_OPTIONS":
		return clix.CodeInvalidArgs
	default:
		return clix.CodeUnknown
	}
}
