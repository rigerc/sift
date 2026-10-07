// Package provider holds pure helpers shared by the HTTP discovery adapters.
// It has no network or state dependencies.
package provider

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"go-s/internal/backend"
)

const (
	// DefaultLimit matches the small result window discovery uses.
	DefaultLimit = 10
	// MaxLimit caps a provider request regardless of the caller's Limit.
	MaxLimit = 50
)

var (
	repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	slugPattern = regexp.MustCompile(`[^a-z0-9._]+`)
)

// SearchQuery builds the provider query: the primary unresolved term plus the
// single highest-confidence context term, sanitized for safe query strings.
func SearchQuery(q backend.Query) string {
	terms := backend.QueryTerms(q)
	if len(terms) > 2 {
		terms = terms[:2]
	}
	return backend.SanitizeQuery(terms)
}

// Limit clamps a requested result count into the provider-safe window.
func Limit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// Usable reports whether a derived source or name may be handed downstream.
// Names starting with "-" could be read as flags and control characters could
// corrupt terminal/CLI output, so both are rejected here.
func Usable(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

// Source normalizes provider whitespace without hiding hostile controls.
// It does not establish that a source is trusted or available; plans perform
// structural validation before emitting commands.
func Source(raw string) string {
	if strings.ContainsFunc(raw, unicode.IsControl) {
		return ""
	}
	s := strings.TrimSpace(raw)
	if !Usable(s) {
		return ""
	}
	return s
}

// Name reduces a provider name or id to a slash-free, colon-free segment and
// reports "" when nothing usable remains.
func Name(name string) string {
	if strings.ContainsFunc(name, unicode.IsControl) {
		return ""
	}
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, "/:"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if !Usable(name) {
		return ""
	}
	return name
}

// RepoFromURL extracts a GitHub owner/repo from a plain "owner/repo" value or
// from a github.com URL. It returns "" when the value is not a repository.
func RepoFromURL(raw string) string {
	if strings.ContainsFunc(raw, unicode.IsControl) {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if repoPattern.MatchString(raw) {
		return strings.ToLower(raw)
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), "github.com") || u.User != nil || strings.ContainsFunc(u.Path, unicode.IsControl) {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(parts[0] + "/" + parts[1])
}

// Slug lowercases a display string into a flag-safe token.
func Slug(s string) string {
	if strings.ContainsFunc(s, unicode.IsControl) {
		return ""
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugPattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, ".-")
	if !Usable(s) {
		return ""
	}
	return s
}

// Clamp01 bounds a provider score to the 0..1 range the registry expects.
func Clamp01(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// RankScore converts a zero-based result position into a descending 0..1 score.
func RankScore(rank int) float64 {
	if rank < 0 {
		return 0
	}
	return 1.0 / (1.0 + float64(rank))
}
