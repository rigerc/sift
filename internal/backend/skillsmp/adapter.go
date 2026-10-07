// Package skillsmp adapts the SkillsMP skill search API (skillsmp.com) to the
// backend.Searcher contract. SkillsMP is search-only and accepts an optional
// bearer token for higher quotas.
package skillsmp

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/backend/httpx"
	"github.com/rigerc/sift/internal/backend/provider"
	"github.com/rigerc/sift/internal/model"
)

// DefaultBase is the SkillsMP origin.
const DefaultBase = "https://skillsmp.com"

type (
	Adapter struct {
		name   string
		client httpx.Client
	}

	searchResponse struct {
		Success bool           `json:"success"`
		Data    searchData     `json:"data"`
		Error   *providerError `json:"error"`
	}
	searchData struct {
		Skills     []skill    `json:"skills"`
		Pagination pagination `json:"pagination"`
		Usage      usage      `json:"usage"`
	}
	pagination struct {
		Page  int `json:"page"`
		Total int `json:"total"`
	}
	usage struct {
		Plan      string `json:"plan"`
		Remaining int    `json:"remaining"`
	}
	providerError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	skill struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Stars       int    `json:"stars"`
		GithubURL   string `json:"githubUrl"`
		SkillURL    string `json:"skillUrl"`
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

// Search queries SkillsMP's skill search endpoint.
func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := provider.SearchQuery(q)
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	resp, err := a.fetch(ctx, query, provider.Limit(q.Limit))
	if err != nil {
		return nil, err
	}
	return a.mapResults(resp.Data.Skills), nil
}

// Probe runs a minimal search and reports the account plan/quota when exposed.
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	resp, err := a.fetch(ctx, "skill", 1)
	if err != nil {
		return "", err
	}
	parts := []string{}
	if plan := strings.TrimSpace(resp.Data.Usage.Plan); plan != "" {
		parts = append(parts, "plan "+plan)
	}
	if resp.Data.Usage.Remaining > 0 {
		parts = append(parts, fmt.Sprintf("%d remaining", resp.Data.Usage.Remaining))
	}
	if len(parts) == 0 {
		return "online", nil
	}
	return strings.Join(parts, ", "), nil
}

func (a *Adapter) fetch(ctx context.Context, query string, limit int) (searchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("page", "1")
	params.Set("limit", strconv.Itoa(limit))
	params.Set("sortBy", "relevance")

	var resp searchResponse
	if err := a.client.GetJSON(ctx, "/api/v1/skills/search", params, &resp); err != nil {
		return searchResponse{}, err
	}
	if !resp.Success {
		if resp.Error != nil {
			return searchResponse{}, fmt.Errorf("skillsmp: %s: %s", resp.Error.Code, resp.Error.Message)
		}
		return searchResponse{}, fmt.Errorf("skillsmp: request failed")
	}
	return resp, nil
}

func (a *Adapter) mapResults(skills []skill) []backend.ExternalSuggestion {
	out := make([]backend.ExternalSuggestion, 0, len(skills))
	seen := map[string]bool{}
	for rank, skill := range skills {
		source := sourceFor(skill)
		name := provider.Name(skill.Name)
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
			Title:         strings.TrimSpace(skill.Description),
			URL:           urlFor(skill),
			Reason:        reasonFor(skill),
			SourceBackend: a.name,
			ExternalScore: provider.RankScore(rank),
			Stale:         false,
		})
	}
	return out
}

// sourceFor prefers an installable GitHub owner/repo parsed from githubUrl and
// falls back to the SkillsMP skill page.
func sourceFor(skill skill) string {
	if repo := provider.RepoFromURL(skill.GithubURL); repo != "" {
		return repo
	}
	return provider.Source(skill.SkillURL)
}

func urlFor(skill skill) string {
	for _, candidate := range []string{skill.SkillURL, skill.GithubURL} {
		if ref := strings.TrimSpace(candidate); ref != "" {
			return ref
		}
	}
	return ""
}

func reasonFor(skill skill) string {
	reason := strings.TrimSpace(skill.Author)
	if skill.Stars > 0 {
		if reason != "" {
			reason += " · ★" + strconv.Itoa(skill.Stars)
		} else {
			reason = "★" + strconv.Itoa(skill.Stars)
		}
	}
	return reason
}
