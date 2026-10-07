// Package skyll adapts the Skyll skill search API (api.skyll.app) to the
// backend.Searcher contract. Skyll is search-only; results prefer a GitHub
// owner/repo install source and fall back to the provider's skill page.
package skyll

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/backend/httpx"
	"github.com/rigerc/sift/internal/backend/provider"
	"github.com/rigerc/sift/internal/model"
)

// DefaultBase is the Skyll API origin.
const DefaultBase = "https://api.skyll.app"

type (
	Adapter struct {
		name   string
		client httpx.Client
	}

	searchResponse struct {
		Query  string       `json:"query"`
		Count  int          `json:"count"`
		Skills []skyllSkill `json:"skills"`
	}
	skyllSkill struct {
		ID             string    `json:"id"`
		Title          string    `json:"title"`
		Description    string    `json:"description"`
		Source         string    `json:"source"`
		RelevanceScore float64   `json:"relevance_score"`
		Refs           skyllRefs `json:"refs"`
	}
	skyllRefs struct {
		GitHub   string `json:"github"`
		SkillsSh string `json:"skills_sh"`
		Raw      string `json:"raw"`
	}
)

// New builds the adapter. cfg.URL overrides the API origin and cfg.Auth is an
// optional bearer token.
func New(cfg backend.Config) (backend.Backend, error) {
	base := cfg.URL
	if base == "" {
		base = DefaultBase
	}
	return &Adapter{name: cfg.Name, client: httpx.Client{BaseURL: base, Auth: cfg.Auth}}, nil
}

func (a *Adapter) Name() string { return a.name }

// Search queries Skyll's /search endpoint. An empty query returns no results
// without issuing a request.
func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := provider.SearchQuery(q)
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", strconv.Itoa(provider.Limit(q.Limit)))
	params.Set("include_content", "false")
	params.Set("include_raw", "false")
	params.Set("include_references", "false")

	var resp searchResponse
	if err := a.client.GetJSON(ctx, "/search", params, &resp); err != nil {
		return nil, err
	}
	return a.mapResults(resp.Skills), nil
}

// Probe checks the Skyll health endpoint.
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	if _, err := a.client.Get(ctx, "/health", nil); err != nil {
		return "", err
	}
	return "healthy", nil
}

func (a *Adapter) mapResults(skills []skyllSkill) []backend.ExternalSuggestion {
	out := make([]backend.ExternalSuggestion, 0, len(skills))
	seen := map[string]bool{}
	for rank, skill := range skills {
		source := sourceFor(skill)
		name := nameFor(skill)
		if !provider.Usable(source) || name == "" {
			continue
		}
		ref := model.SkillRef{Source: source, Name: name}
		if seen[ref.Key()] {
			continue
		}
		seen[ref.Key()] = true
		out = append(out, backend.ExternalSuggestion{
			Skill:         ref,
			Title:         titleFor(skill),
			URL:           urlFor(skill),
			Reason:        reasonFor(skill),
			SourceBackend: a.name,
			ExternalScore: scoreFor(skill, rank),
			Stale:         false,
		})
	}
	return out
}

// sourceFor prefers an installable GitHub owner/repo: the provider's own
// source field, then a repo parsed from refs.github. It falls back to the
// skill detail page so non-GitHub results remain visible.
func sourceFor(skyllSkill skyllSkill) string {
	source := skyllSkill.Source
	if repo := provider.RepoFromURL(source); repo != "" {
		return repo
	}
	if repo := provider.RepoFromURL(skyllSkill.Refs.GitHub); repo != "" {
		return repo
	}
	if ref := provider.Source(skyllSkill.Refs.SkillsSh); ref != "" {
		return ref
	}
	return provider.Source(skyllSkill.Refs.GitHub)
}

func nameFor(skyllSkill skyllSkill) string {
	if name := provider.Name(skyllSkill.ID); name != "" {
		return name
	}
	return provider.Slug(skyllSkill.Title)
}

func titleFor(skyllSkill skyllSkill) string {
	if title := strings.TrimSpace(skyllSkill.Title); title != "" {
		return title
	}
	return strings.TrimSpace(skyllSkill.Description)
}

func urlFor(skyllSkill skyllSkill) string {
	for _, candidate := range []string{skyllSkill.Refs.SkillsSh, skyllSkill.Refs.GitHub, skyllSkill.Refs.Raw} {
		if ref := strings.TrimSpace(candidate); ref != "" {
			return ref
		}
	}
	return ""
}

func reasonFor(skyllSkill skyllSkill) string {
	if source := strings.TrimSpace(skyllSkill.Source); source != "" {
		return source
	}
	return "Skyll"
}

func scoreFor(skyllSkill skyllSkill, rank int) float64 {
	if skyllSkill.RelevanceScore > 0 {
		return provider.Clamp01(skyllSkill.RelevanceScore)
	}
	return provider.RankScore(rank)
}
