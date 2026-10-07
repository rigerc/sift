// Package app provides UI-independent application operations.
package app

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"

	"go-s/internal/backend"
	"go-s/internal/backend/register"
	"go-s/internal/detect"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/resolve"
	"go-s/internal/rules"
	"go-s/internal/walk"
)

type Service struct {
	Runner install.Runner
	Output io.Writer
	// RegistryBuilder constructs the discovery registry; tests inject fakes.
	// nil selects the built-in CLI adapter registry.
	RegistryBuilder func() (*backend.Registry, error)
}
type ScanOptions struct {
	Catalog  string
	MaxDepth int
	Online   bool
}

func (s Service) Scan(ctx context.Context, root string, opts ScanOptions) (model.ScanResult, error) {
	catalog, err := rules.Load(opts.Catalog)
	if err != nil {
		return model.ScanResult{}, err
	}
	manifest, err := walk.Run(ctx, root, walk.Options{MaxDepth: opts.MaxDepth})
	if err != nil {
		return model.ScanResult{}, err
	}
	detection, err := detect.Run(ctx, manifest, catalog.DetectionRules)
	if err != nil {
		return model.ScanResult{}, err
	}
	members := []string{}
	for _, m := range manifest.Members {
		if m.Root != "." {
			members = append(members, m.Root)
		}
	}
	signals := detect.Merge(detection.Signals, len(members))
	for i := range signals {
		for _, tech := range catalog.Technologies {
			if tech.ID == signals[i].Key {
				signals[i].Domain = tech.Domain
				break
			}
		}
	}
	resolved, err := resolve.Run(signals, detection.Unresolved, catalog)
	if err != nil {
		return model.ScanResult{}, err
	}
	warnings := append([]string{}, detection.Warnings...)
	suggestions := resolved.Suggestions
	if opts.Online {
		external, backendWarnings := s.discover(ctx, resolved.Unresolved, signals)
		suggestions = append(suggestions, external...)
		warnings = append(warnings, backendWarnings...)
	}
	sortSuggestionsByScore(suggestions)
	slices.Sort(warnings)
	warnings = slices.Compact(warnings)
	return model.ScanResult{Root: manifest.Root, Members: members, Observations: detection.Observations, Signals: signals, ResolveResult: model.ResolveResult{Suggestions: suggestions, Unresolved: resolved.Unresolved, Order: resolved.Order}, Warnings: warnings}, nil
}

const (
	discoveryObservationCap = 4
	discoveryContextCap     = 3
	discoveryPerQueryLimit  = 8
	discoveryResultLimit    = 20
	discoveryRRFK           = 60.0
)

// discover performs focused external discovery. Each unresolved observation is
// searched independently with a small amount of high-confidence local context,
// then the result lists are fused. This avoids the old "kitchen sink" query in
// which unrelated package names diluted one another.
//
// Individual backend failures degrade to warnings; they never fail the scan.
// Results land in the external bucket: never auto-selected, never installed
// by --yes, and only installed after explicit user selection.
func (s Service) discover(ctx context.Context, unresolved []model.Observation, signals []model.MergedSignal) ([]model.Suggestion, []string) {
	unresolved = prioritizeDiscoveryObservations(unresolved, discoveryObservationCap)
	if len(unresolved) == 0 {
		return nil, nil
	}
	signals = prioritizeDiscoveryContext(signals, discoveryContextCap)

	build := s.RegistryBuilder
	if build == nil {
		build = register.New
	}
	registry, err := build()
	if err != nil {
		return nil, []string{"backends: " + err.Error()}
	}

	type answer struct {
		i     int
		found []backend.ExternalSuggestion
		errs  []error
	}
	answers := make([]answer, len(unresolved))
	var wg sync.WaitGroup
	for i, observation := range unresolved {
		wg.Add(1)
		go func(i int, observation model.Observation) {
			defer wg.Done()
			found, errs := registry.Search(ctx, backend.Query{
				Unresolved: []model.Observation{observation},
				Context:    signals,
				Limit:      discoveryPerQueryLimit,
			})
			answers[i] = answer{i: i, found: found, errs: errs}
		}(i, observation)
	}
	wg.Wait()

	warnings := []string{}
	lists := make([][]backend.ExternalSuggestion, len(answers))
	for _, answer := range answers {
		lists[answer.i] = answer.found
		for _, err := range answer.errs {
			warnings = append(warnings, "backend discovery: "+err.Error())
		}
	}

	found := fuseDiscoveryResults(lists, discoveryResultLimit)
	out := make([]model.Suggestion, 0, len(found))
	for _, v := range found {
		reason := v.Reason
		if reason == "" {
			reason = "matched by " + v.SourceBackend
		}
		var evidence []string
		if v.Title != "" {
			evidence = []string{v.Title}
		}
		out = append(out, model.Suggestion{
			Skill:         v.Skill,
			Bucket:        "external",
			Reasons:       []string{reason},
			Evidence:      evidence,
			SourceBackend: v.SourceBackend,
			ExternalScore: v.ExternalScore,
			URL:           v.URL,
			Stale:         v.Stale,
		})
	}
	return out, warnings
}

type discoveryObservationRank struct {
	observation model.Observation
	term        string
	occurrences int
	root        bool
}

// prioritizeDiscoveryObservations removes repeated observations (common in
// monorepos) and chooses the most informative external-search triggers first.
// Package observations outrank config and content observations; repeated and
// root-level observations then receive preference.
func prioritizeDiscoveryObservations(in []model.Observation, limit int) []model.Observation {
	byTerm := map[string]*discoveryObservationRank{}
	for _, observation := range in {
		term := discoveryObservationTerm(observation)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		candidate := byTerm[key]
		if candidate == nil {
			candidate = &discoveryObservationRank{observation: observation, term: term}
			byTerm[key] = candidate
		}
		candidate.occurrences++
		if observation.Member == "" || observation.Member == "." {
			candidate.root = true
			candidate.observation = observation
		}
		if observationKindPriority(observation.Kind) > observationKindPriority(candidate.observation.Kind) {
			candidate.observation = observation
		}
	}

	ranked := make([]*discoveryObservationRank, 0, len(byTerm))
	for _, item := range byTerm {
		ranked = append(ranked, item)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if pa, pb := observationKindPriority(a.observation.Kind), observationKindPriority(b.observation.Kind); pa != pb {
			return pa > pb
		}
		if a.occurrences != b.occurrences {
			return a.occurrences > b.occurrences
		}
		if a.root != b.root {
			return a.root
		}
		if a.term != b.term {
			return a.term < b.term
		}
		return a.observation.Key < b.observation.Key
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]model.Observation, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.observation)
	}
	return out
}

func discoveryObservationTerm(observation model.Observation) string {
	if value := strings.TrimSpace(observation.Value); value != "" {
		return value
	}
	key := strings.TrimSpace(observation.Key)
	if i := strings.LastIndex(key, ":"); i >= 0 && i+1 < len(key) {
		key = key[i+1:]
	}
	return strings.TrimSpace(key)
}

func observationKindPriority(kind model.ObservationKind) int {
	switch kind {
	case model.ObsPackage:
		return 3
	case model.ObsConfig:
		return 2
	case model.ObsContent:
		return 1
	default:
		return 0
	}
}

// prioritizeDiscoveryContext makes context selection relevance-driven rather
// than dependent on the resolver's deterministic domain/key ordering.
func prioritizeDiscoveryContext(in []model.MergedSignal, limit int) []model.MergedSignal {
	out := slices.Clone(in)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		if a.RootObserved != b.RootObserved {
			return a.RootObserved
		}
		if len(a.Members) != len(b.Members) {
			return len(a.Members) > len(b.Members)
		}
		return a.Key < b.Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// fuseDiscoveryResults uses reciprocal-rank fusion across the focused queries.
// A skill returned for multiple unresolved observations rises naturally, while
// raw backend scores remain available for display but are not treated as
// directly comparable across providers.
func fuseDiscoveryResults(lists [][]backend.ExternalSuggestion, limit int) []backend.ExternalSuggestion {
	type fused struct {
		value backend.ExternalSuggestion
		hits  int
		rrf   float64
	}
	byKey := map[string]*fused{}
	for _, list := range lists {
		seenInQuery := map[string]bool{}
		for rank, value := range list {
			if value.Skill.Source == "" || value.Skill.Name == "" {
				continue
			}
			key := value.Skill.Key()
			if seenInQuery[key] {
				continue
			}
			seenInQuery[key] = true
			item := byKey[key]
			if item == nil {
				copy := value
				item = &fused{value: copy}
				byKey[key] = item
			} else {
				if value.ExternalScore > item.value.ExternalScore {
					item.value.ExternalScore = value.ExternalScore
				}
				item.value.Stale = item.value.Stale && value.Stale
				if item.value.Title == "" {
					item.value.Title = value.Title
				}
				if item.value.Reason == "" {
					item.value.Reason = value.Reason
				}
			}
			item.hits++
			item.rrf += 1.0 / (discoveryRRFK + float64(rank+1))
		}
	}

	ranked := make([]*fused, 0, len(byKey))
	for _, item := range byKey {
		ranked = append(ranked, item)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.hits != b.hits {
			return a.hits > b.hits
		}
		if a.rrf != b.rrf {
			return a.rrf > b.rrf
		}
		if a.value.Stale != b.value.Stale {
			return !a.value.Stale
		}
		if a.value.ExternalScore != b.value.ExternalScore {
			return a.value.ExternalScore > b.value.ExternalScore
		}
		return a.value.Skill.Key() < b.value.Skill.Key()
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]backend.ExternalSuggestion, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].value
	}
	return out
}

// suggestionScore is the display rank of a suggestion: backend-provided
// scores for external results, resolver confidence otherwise. The two scales
// are both 0..1 (external scores are clamped at merge time).
func suggestionScore(s model.Suggestion) float64 {
	if s.Bucket == "external" {
		return s.ExternalScore
	}
	return s.Confidence
}

// sortSuggestionsByScore orders the merged scan results globally by score,
// highest first, with a canonical skill-key tiebreak so output stays
// deterministic. Resolver install order is preserved separately in Order.
func sortSuggestionsByScore(suggestions []model.Suggestion) {
	sort.SliceStable(suggestions, func(i, j int) bool {
		a, b := suggestions[i], suggestions[j]
		if sa, sb := suggestionScore(a), suggestionScore(b); sa != sb {
			return sa > sb
		}
		return a.Skill.Key() < b.Skill.Key()
	})
}

// PlanInstall restores scan-result order after UI selection. A nil selection picks suggested locals.
func (s Service) PlanInstall(root string, result model.ScanResult, selected []model.SkillRef, opts install.Options) (install.Plan, error) {
	chosen := map[string]bool{}
	if selected == nil {
		for _, suggestion := range result.Suggestions {
			if suggestion.Bucket == "suggested" {
				chosen[suggestion.Skill.Key()] = true
			}
		}
	} else {
		for _, ref := range selected {
			chosen[ref.Key()] = true
		}
	}
	refs := []model.SkillRef{}
	for _, suggestion := range result.Suggestions {
		if chosen[suggestion.Skill.Key()] {
			refs = append(refs, suggestion.Skill)
			delete(chosen, suggestion.Skill.Key())
		}
	}
	if len(chosen) > 0 {
		return install.Plan{}, fmt.Errorf("selection includes skills outside scan results")
	}
	return install.Build(root, refs, opts)
}

func (s Service) Install(ctx context.Context, plan install.Plan) error {
	return install.Execute(ctx, plan, s.Runner, s.Output)
}
func (s Service) Status(root string) ([]install.Status, error) { return install.Inspect(root) }
func (s Service) Update(ctx context.Context, root string) error {
	return install.Update(ctx, root, s.Runner, s.Output)
}
