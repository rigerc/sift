package backend

import (
	"sort"
	"strings"
)

const (
	maxQueryTerms = 4
	maxQueryLen   = 256
)

// QueryTerms builds a compact external-search query. Unresolved observations
// are the primary terms; only the strongest known signals are added as context.
// Terms are de-duplicated case-insensitively and technology-key namespaces such
// as "node:react" are reduced to the useful search token "react".
func QueryTerms(q Query) []string {
	out := make([]string, 0, maxQueryTerms)
	seen := map[string]bool{}
	add := func(term string) {
		if len(out) >= maxQueryTerms {
			return
		}
		term = simplifyQueryTerm(term)
		key := strings.ToLower(term)
		if term == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, term)
	}

	for _, o := range q.Unresolved {
		term := o.Value
		if strings.TrimSpace(term) == "" {
			term = o.Key
		}
		add(term)
	}

	type contextTerm struct {
		key        string
		confidence float64
	}
	ctx := make([]contextTerm, 0, len(q.Context))
	for _, s := range q.Context {
		ctx = append(ctx, contextTerm{key: s.Key, confidence: s.Confidence})
	}
	sort.SliceStable(ctx, func(i, j int) bool {
		if ctx[i].confidence != ctx[j].confidence {
			return ctx[i].confidence > ctx[j].confidence
		}
		return ctx[i].key < ctx[j].key
	})
	for _, s := range ctx {
		add(s.key)
	}
	return out
}

// simplifyQueryTerm keeps search input human-shaped without weakening the
// argument sanitizer. Detector keys are namespaced with colons; external CLIs
// generally rank the leaf token better than the internal key.
func simplifyQueryTerm(term string) string {
	term = strings.TrimSpace(term)
	if i := strings.LastIndex(term, ":"); i >= 0 && i+1 < len(term) {
		term = term[i+1:]
	}
	return strings.TrimSpace(term)
}

// SanitizeQuery collapses a multi-term search into one sanitized argument,
// blocking flag injection by stripping anything that could start a flag.
func SanitizeQuery(terms []string) string {
	cleaned := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		term = strings.TrimLeft(term, "-")
		if term == "" {
			continue
		}
		cleaned = append(cleaned, term)
	}
	joined := strings.Join(cleaned, " ")
	if len(joined) > maxQueryLen {
		joined = joined[:maxQueryLen]
	}
	return joined
}
