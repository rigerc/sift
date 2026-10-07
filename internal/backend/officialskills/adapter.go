// Package officialskills adapts the officialskills registry
// (github.com/rigerc/officialskills) to the backend.Searcher contract.
//
// Every suggestion it returns is installable with `npx skills add <repo>`
// and carries the skill's canonical GitHub URL.
package officialskills

import (
	"context"
	"fmt"
	"go-s/internal/backend"
	"go-s/internal/model"
	"sort"
	"strings"
)

// Adapter implements backend.Searcher over the merged registry snapshot.
type Adapter struct {
	name   string
	loader *Loader
}

// New builds the adapter. cfg.URL overrides the registry base and cfg.CacheDir
// overrides the on-disk cache location.
func New(cfg backend.Config) (backend.Backend, error) {
	return &Adapter{name: cfg.Name, loader: NewLoader(cfg.URL, cfg.CacheDir)}, nil
}

func (a *Adapter) Name() string { return a.name }

// Probe loads the registry and reports its size, used by `backends check`.
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	snapshot, err := a.loader.Load(ctx)
	if err != nil {
		return "", err
	}
	state := "online"
	if snapshot.Offline {
		state = "cached"
	}
	return fmt.Sprintf("%d installable skills (%s)", len(snapshot.Skills), state), nil
}

// Search matches the query terms against the installable registry and fuses
// the per-term rankings. Every result has a GitHub repo source and URL.
func (a *Adapter) Search(ctx context.Context, q backend.Query) ([]backend.ExternalSuggestion, error) {
	terms := backend.QueryTerms(q)
	if len(terms) == 0 {
		return []backend.ExternalSuggestion{}, nil
	}
	snapshot, err := a.loader.Load(ctx)
	if err != nil {
		return nil, err
	}
	if len(snapshot.Skills) == 0 {
		return []backend.ExternalSuggestion{}, nil
	}

	type hit struct {
		skill Skill
		rank  int
	}
	byKey := map[string]*hit{}
	order := []string{}
	for _, term := range terms {
		for rank, skill := range Search(snapshot.Skills, term, 0) {
			key := skill.Repo() + "\x00" + skill.Name
			existing := byKey[key]
			if existing == nil {
				byKey[key] = &hit{skill: skill, rank: rank}
				order = append(order, key)
				continue
			}
			if rank < existing.rank {
				existing.rank = rank
			}
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return order[i] < order[j]
	})
	if q.Limit > 0 && len(order) > q.Limit {
		order = order[:q.Limit]
	}

	out := make([]backend.ExternalSuggestion, 0, len(order))
	for _, key := range order {
		h := byKey[key]
		skill := h.skill
		out = append(out, backend.ExternalSuggestion{
			Skill:         model.SkillRef{Source: skill.Repo(), Name: skill.Name},
			URL:           skill.URL,
			Title:         skill.Description,
			Reason:        reasonFor(skill),
			SourceBackend: a.name,
			ExternalScore: 1.0 / (1.0 + float64(h.rank)),
			Stale:         snapshot.Offline,
		})
	}
	return out, nil
}

func reasonFor(s Skill) string {
	parts := []string{}
	if p := strings.TrimSpace(s.Publisher); p != "" {
		parts = append(parts, p)
	}
	if section := strings.TrimSpace(s.Section); section != "" {
		parts = append(parts, section)
	}
	return strings.Join(parts, " · ")
}
