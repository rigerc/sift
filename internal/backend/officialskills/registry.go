package officialskills

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go-s/internal/backend/provider"
)

// Skill is one entry from official.json / community.json `skills[]`.
type Skill struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	Publisher   string  `json:"publisher"`
	Section     string  `json:"section"`
	RepoPtr     *string `json:"repo"`
}

// Repo returns the skill's GitHub owner/repo, or "" when unresolved.
func (s Skill) Repo() string {
	if s.RepoPtr == nil {
		return ""
	}
	return provider.Source(*s.RepoPtr)
}

// Installable reports whether the skill can be installed via `npx skills`,
// which requires a resolved GitHub repository.
func (s Skill) Installable() bool { return s.Repo() != "" }

// Output is the top-level registry file (official.json / community.json).
type Output struct {
	Meta struct {
		FetchedAt   string `json:"fetched_at"`
		Source      string `json:"source"`
		TotalSkills int    `json:"total_skills"`
	} `json:"meta"`
	Skills []Skill `json:"skills"`
}

// ParseOutput decodes one registry JSON document.
func ParseOutput(data []byte) (Output, error) {
	var out Output
	if err := json.Unmarshal(data, &out); err != nil {
		return Output{}, fmt.Errorf("parse registry JSON: %w", err)
	}
	if out.Skills == nil {
		return Output{}, fmt.Errorf("parse registry JSON: missing skills array")
	}
	return out, nil
}

// Flatten merges flat skill lists from multiple sources, preserving order.
func Flatten(outputs ...Output) []Skill {
	var all []Skill
	for _, o := range outputs {
		all = append(all, o.Skills...)
	}
	return all
}

// InstallableOnly returns the subset that can be installed via `npx skills`.
func InstallableOnly(skills []Skill) []Skill {
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if s.Installable() && strings.TrimSpace(s.URL) != "" {
			out = append(out, s)
		}
	}
	return out
}

// Search scores skills against a query over id/name/description/publisher/repo.
// Exact id prefix ranks first, then name prefix, then substring anywhere. The
// returned order is deterministic for a stable input order.
func Search(skills []Skill, query string, limit int) []Skill {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		if limit > 0 && len(skills) > limit {
			return skills[:limit]
		}
		return skills
	}
	type scored struct {
		s     Skill
		score int
	}
	var ranked []scored
	for _, s := range skills {
		id := strings.ToLower(s.ID)
		name := strings.ToLower(s.Name)
		desc := strings.ToLower(s.Description)
		pub := strings.ToLower(s.Publisher)
		repo := strings.ToLower(s.Repo())
		score := -1
		switch {
		case id == query:
			score = 0
		case strings.HasPrefix(id, query):
			score = 1
		case name == query:
			score = 2
		case strings.HasPrefix(name, query):
			score = 3
		case strings.Contains(id, query):
			score = 4
		case strings.Contains(name, query):
			score = 5
		case strings.Contains(pub, query):
			score = 6
		case strings.Contains(repo, query):
			score = 7
		case strings.Contains(desc, query):
			score = 8
		}
		if score >= 0 {
			ranked = append(ranked, scored{s: s, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score < ranked[j].score })
	out := make([]Skill, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.s)
	}
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}
