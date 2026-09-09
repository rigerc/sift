// Package resolve performs deterministic local technology-to-skill resolution.
package resolve

import (
	"fmt"
	"go-s/internal/model"
	"go-s/internal/rules"
	"sort"
	"strings"
)

type suggestionState struct {
	model.Suggestion
	contributors map[string]struct{}
}

// Run resolves known signals through local rules and retains externally
// eligible observations for the later discovery stage.
func Run(signals []model.MergedSignal, observations []model.Observation, catalog rules.Catalog) (model.ResolveResult, error) {
	states := make(map[string]*suggestionState)
	byTechnology := make(map[string]model.MergedSignal, len(signals))
	for _, signal := range signals {
		byTechnology[signal.Key] = signal
		if rule, ok := catalog.SkillRule(signal.Key); ok {
			confidence := rule.RuleConfidence
			if confidence == 0 {
				confidence = 1
			}
			for _, skill := range rule.Skills {
				add(states, skill, signal.Confidence*confidence, append(append([]string{}, signal.Reasons...), "technology "+signal.Key), signal.Evidence, []string{signal.Key}, signal.Members)
			}
		}
	}

	edges := make(map[string]map[string]struct{})
	for _, combo := range catalog.ComboRules {
		if !comboMatches(combo, byTechnology) {
			continue
		}
		confidence := 1.0
		for _, trigger := range combo.Triggers {
			if byTechnology[trigger].Confidence < confidence {
				confidence = byTechnology[trigger].Confidence
			}
		}
		reasons := []string{"combo " + combo.ID}
		var evidence, members, technologies []string
		for _, trigger := range combo.Triggers {
			signal := byTechnology[trigger]
			technologies = append(technologies, trigger)
			evidence = append(evidence, signal.Evidence...)
			members = append(members, signal.Members...)
		}
		for _, skill := range combo.Skills {
			add(states, skill, confidence, reasons, evidence, technologies, members)
		}
		for i := 1; i < len(combo.Order); i++ {
			from, to := combo.Order[i-1].Key(), combo.Order[i].Key()
			if from == to {
				continue
			}
			if edges[from] == nil {
				edges[from] = make(map[string]struct{})
			}
			edges[from][to] = struct{}{}
		}
	}

	keys := make([]string, 0, len(states))
	for key, state := range states {
		state.Bucket = bucket(state.Confidence)
		state.Reasons = uniqueSorted(state.Reasons)
		state.Evidence = uniqueSorted(state.Evidence)
		state.Technologies = uniqueSorted(state.Technologies)
		state.Members = uniqueSorted(state.Members)
		keys = append(keys, key)
	}
	order, err := topoOrder(keys, states, edges)
	if err != nil {
		return model.ResolveResult{}, err
	}
	suggestions := make([]model.Suggestion, 0, len(order))
	for _, key := range order {
		suggestions = append(suggestions, states[key].Suggestion)
	}

	unresolved := make([]model.Observation, 0, len(observations))
	for _, observation := range observations {
		if externallyEligible(observation) {
			unresolved = append(unresolved, observation)
		}
	}
	sort.Slice(unresolved, func(i, j int) bool {
		a, b := unresolved[i], unresolved[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Member != b.Member {
			return a.Member < b.Member
		}
		return strings.Join(a.Evidence, "\x00") < strings.Join(b.Evidence, "\x00")
	})
	return model.ResolveResult{Suggestions: suggestions, Unresolved: unresolved, Order: refs(order, states)}, nil
}

func add(states map[string]*suggestionState, skill model.SkillRef, confidence float64, reasons, evidence, technologies, members []string) {
	key := skill.Key()
	state := states[key]
	if state == nil {
		state = &suggestionState{Suggestion: model.Suggestion{Skill: skill, Confidence: confidence}, contributors: make(map[string]struct{})}
		states[key] = state
	} else if confidence > state.Confidence {
		state.Confidence = confidence
	}
	state.Reasons = append(state.Reasons, reasons...)
	state.Evidence = append(state.Evidence, evidence...)
	state.Technologies = append(state.Technologies, technologies...)
	state.Members = append(state.Members, members...)
}

func comboMatches(combo model.ComboRule, signals map[string]model.MergedSignal) bool {
	for _, trigger := range combo.Triggers {
		if _, ok := signals[trigger]; !ok {
			return false
		}
	}
	for _, conflict := range combo.Conflicts {
		if _, ok := signals[conflict]; ok {
			return false
		}
	}
	return len(combo.Triggers) > 0
}

func bucket(confidence float64) string {
	if confidence >= 0.7 {
		return "suggested"
	}
	if confidence >= 0.4 {
		return "possible"
	}
	return "hidden"
}

func topoOrder(keys []string, states map[string]*suggestionState, edges map[string]map[string]struct{}) ([]string, error) {
	known := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		known[key] = struct{}{}
	}
	graphNodes := make(map[string]struct{})
	for from, tos := range edges {
		if _, ok := known[from]; ok {
			graphNodes[from] = struct{}{}
		}
		for to := range tos {
			if _, ok := known[to]; ok {
				graphNodes[to] = struct{}{}
			}
		}
	}
	indegree := make(map[string]int, len(keys))
	for _, key := range keys {
		indegree[key] = 0
	}
	for from, tos := range edges {
		if _, ok := known[from]; !ok {
			continue
		}
		for to := range tos {
			if _, ok := known[to]; ok {
				indegree[to]++
			}
		}
	}
	ready := make([]string, 0)
	for key := range graphNodes {
		if indegree[key] == 0 {
			ready = append(ready, key)
		}
	}
	ordered := make([]string, 0, len(graphNodes))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return lessSkill(ready[i], ready[j], states) })
		key := ready[0]
		ready = ready[1:]
		ordered = append(ordered, key)
		for to := range edges[key] {
			if _, ok := indegree[to]; !ok {
				continue
			}
			indegree[to]--
			if indegree[to] == 0 {
				ready = append(ready, to)
			}
		}
	}
	if len(ordered) != len(graphNodes) {
		remaining := make([]string, 0)
		for key, degree := range indegree {
			if degree > 0 {
				remaining = append(remaining, key)
			}
		}
		sort.Strings(remaining)
		return nil, fmt.Errorf("skill ordering cycle: %s", strings.Join(remaining, ", "))
	}
	unordered := make([]string, 0, len(keys)-len(ordered))
	for _, key := range keys {
		if _, ok := graphNodes[key]; !ok {
			unordered = append(unordered, key)
		}
	}
	sort.Slice(unordered, func(i, j int) bool {
		if states[unordered[i]].Confidence != states[unordered[j]].Confidence {
			return states[unordered[i]].Confidence > states[unordered[j]].Confidence
		}
		return unordered[i] < unordered[j]
	})
	ordered = append(ordered, unordered...)
	return ordered, nil
}

func lessSkill(a, b string, states map[string]*suggestionState) bool {
	_ = states
	// The queue is sorted canonically. This makes Kahn's choice stable while
	// the graph itself preserves every combo ordering edge.
	return a < b
}

func refs(keys []string, states map[string]*suggestionState) []model.SkillRef {
	out := make([]model.SkillRef, 0, len(keys))
	for _, key := range keys {
		out = append(out, states[key].Skill)
	}
	return out
}

func externallyEligible(observation model.Observation) bool {
	switch observation.Kind {
	case model.ObsPackage:
		return true
	case model.ObsConfig:
		value := strings.ToLower(observation.Value + " " + observation.Key)
		return strings.Contains(value, "config") || strings.Contains(value, "manifest") || strings.Contains(value, "webpack") || strings.Contains(value, "vite") || strings.Contains(value, "eslint") || strings.Contains(value, "babel")
	case model.ObsContent:
		return strings.TrimSpace(observation.Value) != "" || strings.TrimSpace(observation.Key) != ""
	default:
		return false
	}
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
