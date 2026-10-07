// Package decimalai adapts the DecimalAI registry API (api.decimal.ai) to the
// backend.Searcher contract. DecimalAI is search-only and accepts an optional
// bearer token.
package decimalai

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

const (
	// DefaultBase is the DecimalAI API origin.
	DefaultBase = "https://api.decimal.ai"
	// DetailBase hosts the human-readable skill pages.
	DetailBase = "https://app.decimal.ai"
)

type (
	Adapter struct {
		name   string
		client httpx.Client
	}

	searchResponse struct {
		Items     []skill `json:"items"`
		TotalHint int     `json:"total_hint"`
		Total     int     `json:"total"`
	}
	skill struct {
		URLSlug       string        `json:"url_slug"`
		DisplayName   string        `json:"display_name"`
		Name          string        `json:"name"`
		Description   string        `json:"description"`
		SkillBadge    string        `json:"skill_badge"`
		GitHubURL     string        `json:"github_url"`
		Repo          string        `json:"repo"`
		Repository    string        `json:"repository"`
		SourceURL     string        `json:"source_url"`
		URL           string        `json:"url"`
		Effectiveness effectiveness `json:"effectiveness"`
	}
	effectiveness struct {
		SkillScore float64 `json:"skill_score"`
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

// Search queries the DecimalAI registry search endpoint.
func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	query := provider.SearchQuery(q)
	if query == "" {
		return []backend.ExternalSuggestion{}, nil
	}
	resp, err := a.fetch(ctx, query, provider.Limit(q.Limit))
	if err != nil {
		return nil, err
	}
	return a.mapResults(resp.Items), nil
}

// Probe runs a minimal search and reports the registry's advertised total.
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	resp, err := a.fetch(ctx, "skill", 1)
	if err != nil {
		return "", err
	}
	count := resp.TotalHint
	if count == 0 {
		count = resp.Total
	}
	if count > 0 {
		return fmt.Sprintf("%d skills", count), nil
	}
	return "online", nil
}

func (a *Adapter) fetch(ctx context.Context, query string, limit int) (searchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "recommended")
	params.Set("limit", strconv.Itoa(limit))

	var resp searchResponse
	if err := a.client.GetJSON(ctx, "/api/v1/registry/skills", params, &resp); err != nil {
		return searchResponse{}, err
	}
	return resp, nil
}

func (a *Adapter) mapResults(skills []skill) []backend.ExternalSuggestion {
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
			Title:         strings.TrimSpace(skill.Description),
			URL:           urlFor(skill, source),
			Reason:        reasonFor(skill),
			SourceBackend: a.name,
			ExternalScore: scoreFor(skill, rank),
			Stale:         false,
		})
	}
	return out
}

// githubRef scans DecimalAI's opportunistic repository fields and returns the
// first installable owner/repo plus the field value it came from.
func githubRef(skill skill) (repo, raw string) {
	for _, field := range []string{skill.GitHubURL, skill.Repo, skill.Repository, skill.SourceURL, skill.URL} {
		if strings.TrimSpace(field) == "" {
			continue
		}
		if found := provider.RepoFromURL(field); found != "" {
			return found, strings.TrimSpace(field)
		}
	}
	return "", ""
}

func sourceFor(skill skill) string {
	if repo, _ := githubRef(skill); repo != "" {
		return repo
	}
	if slug := provider.Name(skill.URLSlug); slug != "" {
		return DetailBase + "/skills/" + slug
	}
	return ""
}

func nameFor(skill skill) string {
	if name := provider.Name(skill.URLSlug); name != "" {
		return name
	}
	if name := provider.Slug(skill.DisplayName); name != "" {
		return name
	}
	return provider.Slug(skill.Name)
}

func urlFor(skill skill, source string) string {
	repo, raw := githubRef(skill)
	if raw != "" {
		if strings.HasPrefix(raw, "https://") {
			return raw
		}
		return "https://github.com/" + repo
	}
	if strings.HasPrefix(source, "https://") {
		return source
	}
	return ""
}

func reasonFor(skill skill) string {
	reason := strings.TrimSpace(skill.DisplayName)
	if reason == "" {
		reason = strings.TrimSpace(skill.Name)
	}
	if badge := strings.TrimSpace(skill.SkillBadge); badge != "" {
		if reason != "" {
			reason += " · " + badge
		} else {
			reason = badge
		}
	}
	return reason
}

func scoreFor(skill skill, rank int) float64 {
	score := skill.Effectiveness.SkillScore
	if score <= 0 {
		return provider.RankScore(rank)
	}
	if score > 1 {
		score /= 100
	}
	return provider.Clamp01(score)
}
