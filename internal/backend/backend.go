// Package backend defines capability contracts shared by discovery providers.
package backend

import (
	"context"

	"go-s/internal/model"
)

type (
	Backend  interface{ Name() string }
	Searcher interface {
		Search(context.Context, Query) ([]ExternalSuggestion, error)
	}
)

type Validator interface {
	Validate(context.Context, []model.SkillRef) (map[string]Validation, error)
}

// VersionProber reports a backend health/version summary for `backends check`.
type VersionProber interface {
	Probe(context.Context) (string, error)
}

type Verdict int

const (
	StatusInvalid Verdict = iota
	StatusValid
	StatusUnknown
)

type Validation struct {
	Status    Verdict
	Revision  string
	Downloads int
	Detail    string
}
type Query struct {
	Unresolved []model.Observation
	Context    []model.MergedSignal
	Limit      int
	RulesHash  string
}
type ExternalSuggestion struct {
	Skill                model.SkillRef
	Title, SourceBackend string
	// URL is the skill's canonical detail URL: a GitHub URL when the provider
	// exposes one, otherwise the provider's own skill page. It is display and
	// provenance only; installs use Skill.Source. Skill.Source may be a GitHub
	// owner/repo or an https URL (a GitHub URL or a provider detail page).
	URL           string
	ExternalScore float64
	Reason        string
	Stale         bool
}

type Capability uint8

const (
	CapSearch Capability = 1 << iota
	CapValidate
)

// Config describes one backend instance. URL overrides the backend's remote
// base (e.g. a registry mirror), and CacheDir overrides where a backend keeps
// its local disk cache. Auth carries a secret to redact from logs.
type Config struct {
	Name, Type, URL, Auth string
	CacheDir              string
	Capabilities          Capability
}

func (c Capability) Has(v Capability) bool { return c&v == v }
