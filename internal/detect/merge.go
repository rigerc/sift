package detect

import (
	"slices"
	"sort"

	"github.com/rigerc/sift/internal/model"
)

// Merge combines per-layer signals into deterministic, workspace-scoped
// technology evidence. totalMembers excludes the workspace root.
func Merge(signals []model.Signal, totalMembers int) []model.MergedSignal {
	type scoped struct {
		key, member string
		domain      string
		reasons     map[string]struct{}
		evidence    map[string]struct{}
		layers      map[int]float64
	}
	groups := make(map[string]*scoped)
	for _, signal := range signals {
		member := signal.Member
		if member == "" {
			member = "."
		}
		id := signal.Key + "\x00" + member
		g := groups[id]
		if g == nil {
			g = &scoped{
				key: signal.Key, member: member, domain: signal.Domain,
				reasons: make(map[string]struct{}), evidence: make(map[string]struct{}), layers: make(map[int]float64),
			}
			groups[id] = g
		}
		if signal.Domain != "" && (g.domain == "" || signal.Domain < g.domain) {
			g.domain = signal.Domain
		}
		if signal.Reason != "" {
			g.reasons[signal.Reason] = struct{}{}
		}
		for _, evidence := range signal.Evidence {
			if evidence != "" {
				g.evidence[evidence] = struct{}{}
			}
		}
		base := layerBase(signal.Layer, signal.Kind)
		if base > g.layers[signal.Layer] {
			g.layers[signal.Layer] = base
		}
	}

	byTechnology := make(map[string][]*scoped)
	for _, g := range groups {
		byTechnology[g.key] = append(byTechnology[g.key], g)
	}
	result := make([]model.MergedSignal, 0, len(byTechnology))
	for key, scopes := range byTechnology {
		var root *scoped
		members := make([]*scoped, 0, len(scopes))
		for _, scope := range scopes {
			if scope.member == "." {
				root = scope
			} else {
				members = append(members, scope)
			}
		}
		memberConfidence := 0.0
		if len(members) > 0 {
			for _, member := range members {
				if c := scopeConfidence(member.layers); c > memberConfidence {
					memberConfidence = c
				}
			}
			support := 0.5
			if totalMembers > 0 {
				support += 0.5 * float64(len(members)) / float64(totalMembers)
			}
			memberConfidence *= support
		}
		rootConfidence := 0.0
		if root != nil {
			rootConfidence = scopeConfidence(root.layers)
		}
		confidence := rootConfidence
		if memberConfidence > confidence {
			confidence = memberConfidence
		}
		merged := model.MergedSignal{Key: key, Confidence: confidence, RootObserved: root != nil}
		layerSet := make(map[int]struct{})
		for _, scope := range scopes {
			if scope.member != "." {
				merged.Members = append(merged.Members, scope.member)
			}
			if merged.Domain == "" || (scope.domain != "" && scope.domain < merged.Domain) {
				merged.Domain = scope.domain
			}
			for reason := range scope.reasons {
				merged.Reasons = append(merged.Reasons, reason)
			}
			for evidence := range scope.evidence {
				merged.Evidence = append(merged.Evidence, evidence)
			}
			for layer := range scope.layers {
				layerSet[layer] = struct{}{}
			}
		}
		for layer := range layerSet {
			merged.Layers = append(merged.Layers, layer)
		}
		sort.Strings(merged.Members)
		sort.Strings(merged.Reasons)
		sort.Strings(merged.Evidence)
		merged.Reasons = slices.Compact(merged.Reasons)
		merged.Evidence = slices.Compact(merged.Evidence)
		sort.Ints(merged.Layers)
		result = append(result, merged)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Domain != result[j].Domain {
			return result[i].Domain < result[j].Domain
		}
		return result[i].Key < result[j].Key
	})
	return result
}

func scopeConfidence(layers map[int]float64) float64 {
	highest := 0.0
	for _, base := range layers {
		if base > highest {
			highest = base
		}
	}
	if highest == 0 {
		return 0
	}
	return minFloat(1, highest+0.1*float64(len(layers)-1))
}

func layerBase(layer int, kind model.ObservationKind) float64 {
	switch layer {
	case 2:
		return 0.95
	case 3:
		if kind == model.ObsExt {
			return 0.6
		}
		return 0.85
	case 4:
		return 0.8
	case 5:
		return 0.4
	default:
		return 0
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
