// Package backend defines capability contracts shared by discovery providers.
package backend

import (
	"context"
	"go-s/internal/model"
	"go-s/internal/rules"
	"time"
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
type Reporter interface {
	Report(context.Context, Outcome) error
}
type CatalogProvider interface {
	Catalog(context.Context) (rules.Catalog, error)
}

// VersionProber reports the installed CLI version for local backends.
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
	ExternalScore        float64
	Reason               string
	Stale                bool
}
type Outcome struct {
	Skill    model.SkillRef
	Result   string
	Duration time.Duration
}

type Capability uint8

const (
	CapSearch Capability = 1 << iota
	CapValidate
	CapReport
	CapCatalog
)

type Config struct {
	Name, Type, URL, Auth string
	Capabilities          Capability
	Timeout, CacheTTL     time.Duration
}
type Strategy string

const (
	StrategyFanout   Strategy = "fanout"
	StrategyFirstHit Strategy = "first-hit"
)

func (c Capability) Has(v Capability) bool { return c&v == v }
