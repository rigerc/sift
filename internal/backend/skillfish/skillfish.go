// Package skillfish adapts the skillfish CLI (1.0.39) to the unified Searcher
// contract. The backend documents no formal JSON schema, so parsing is based
// on the observed envelope in the reference (success/exit_code/errors[]) and
// flagged as lower confidence.
package skillfish

import (
	"context"
	"encoding/json"
	"go-s/internal/backend"
	"go-s/internal/backend/clix"
	"go-s/internal/model"
	"strconv"
	"strings"
)

const (
	pinnedVersion = "1.0.39"
	binaryName    = "skillfish"
)

var allowedFlags = []string{"-l", "--json", "--limit"}

type Adapter struct {
	name   string
	binary string
	runner clix.Runner
}

// New builds the adapter. cfg.URL may override the binary path for testing.
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
	Success  bool       `json:"success"`
	ExitCode int        `json:"exit_code"`
	Errors   []apiError `json:"errors"`
	Results  []result   `json:"results"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type result struct {
	Name        string  `json:"name"`
	Repo        string  `json:"repo"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
}

func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := clix.SanitizeQuery(clix.QueryTerms(q))
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	args := []string{"search", query}
	if q.Limit > 0 {
		args = append(args, "-l", strconv.Itoa(q.Limit))
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
		return nil, &clix.Error{Code: clix.CodeUnknown, Message: "skillfish returned unparseable output", Raw: string(stdout)}
	}
	if !payload.Success {
		return nil, a.failure(payload, string(stdout))
	}
	out := make([]backend.ExternalSuggestion, 0, len(payload.Results))
	for _, r := range payload.Results {
		if r.Name == "" {
			continue
		}
		out = append(out, backend.ExternalSuggestion{
			Skill:         model.SkillRef{Source: a.sourceFor(r.Repo), Name: r.Name},
			Title:         r.Description,
			SourceBackend: a.name,
			ExternalScore: r.Score,
		})
	}
	return out, nil
}

// failure prefers the first documented errors[] entry; when errors is empty it
// falls back to the envelope's exit_code as the signal.
func (a *Adapter) failure(payload envelope, raw string) error {
	if len(payload.Errors) > 0 {
		first := payload.Errors[0]
		return &clix.Error{Code: mapAPIError(first.Code), Message: first.Message, Raw: raw}
	}
	return &clix.Error{Code: clix.CodeUnknown, Message: strconv.Itoa(payload.ExitCode) + " without error detail", ExitCode: payload.ExitCode, Raw: raw}
}

func (a *Adapter) sourceFor(repo string) string {
	if repo != "" {
		return repo
	}
	return a.name
}

// mapAPIError maps skillfish error codes heuristically: no formal contract is
// documented, so matching is by substring and defaults to unknown.
func mapAPIError(code string) clix.Code {
	upper := strings.ToUpper(code)
	switch {
	case strings.Contains(upper, "NOT_FOUND"):
		return clix.CodeNotFound
	case strings.Contains(upper, "AUTH"):
		return clix.CodeAuthRequired
	case strings.Contains(upper, "NETWORK"):
		return clix.CodeNetwork
	case strings.Contains(upper, "INVALID"):
		return clix.CodeInvalidArgs
	default:
		return clix.CodeUnknown
	}
}
