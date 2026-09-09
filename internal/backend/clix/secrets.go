package clix

import (
	"go-s/internal/backend"
	"os"
	"strings"
)

// ResolveSecrets turns a backend auth reference (env:NAME, or a literal) into
// the list of strings that must never appear in logs or error output. Missing
// environment variables are skipped: auth failures surface later, per call.
func ResolveSecrets(auth string) []string {
	auth = strings.TrimSpace(auth)
	if auth == "" {
		return nil
	}
	if name, ok := strings.CutPrefix(auth, "env:"); ok {
		if value, found := os.LookupEnv(name); found && value != "" {
			return []string{value}
		}
		return nil
	}
	return []string{auth}
}

// QueryTerms flattens a backend Query into sanitized search terms: unresolved
// observation values first, then context keys.
func QueryTerms(q backend.Query) []string {
	out := make([]string, 0, len(q.Unresolved)+len(q.Context))
	for _, o := range q.Unresolved {
		out = append(out, o.Value)
	}
	for _, s := range q.Context {
		out = append(out, s.Key)
	}
	return out
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
	if len(joined) > maxArgLen {
		joined = joined[:maxArgLen]
	}
	return joined
}
