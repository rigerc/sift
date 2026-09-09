// Package app provides UI-independent application operations.
package app

import (
	"context"
	"fmt"
	"go-s/config"
	"go-s/internal/backend"
	"go-s/internal/backend/register"
	"go-s/internal/detect"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/resolve"
	"go-s/internal/rules"
	"go-s/internal/walk"
	"io"
	"slices"
)

type Service struct {
	Runner install.Runner
	Output io.Writer
	// RegistryBuilder constructs the discovery registry; tests inject fakes.
	// nil selects the built-in adapter registry.
	RegistryBuilder func([]backend.Config, backend.Strategy) (*backend.Registry, error)
}
type ScanOptions struct {
	Catalog      string
	MaxDepth     int
	Online       bool
	BackendsPath string
	ConfigDir    string
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
		external, backendWarnings := s.discover(ctx, opts, detection.Unresolved, signals)
		suggestions = append(suggestions, external...)
		warnings = append(warnings, backendWarnings...)
	}
	slices.Sort(warnings)
	warnings = slices.Compact(warnings)
	return model.ScanResult{Root: manifest.Root, Members: members, Observations: detection.Observations, Signals: signals, ResolveResult: model.ResolveResult{Suggestions: suggestions, Unresolved: resolved.Unresolved, Order: resolved.Order}, Warnings: warnings}, nil
}

const (
	discoveryObservationCap = 8
	discoveryContextCap     = 8
	discoveryResultLimit    = 20
)

// discover fans the query out to the configured discovery backends (rev5 §6).
// Individual backend failures degrade to warnings; they never fail the scan.
// Results land in the external bucket: never auto-selected, never installed
// by --yes, and only installed after explicit user selection.
func (s Service) discover(ctx context.Context, opts ScanOptions, unresolved []model.Observation, signals []model.MergedSignal) ([]model.Suggestion, []string) {
	if len(unresolved) > discoveryObservationCap {
		unresolved = unresolved[:discoveryObservationCap]
	}
	if len(signals) > discoveryContextCap {
		signals = signals[:discoveryContextCap]
	}
	file, _, err := config.LoadBackends(opts.BackendsPath, opts.ConfigDir)
	if err != nil {
		return nil, []string{"backends config: " + err.Error()}
	}
	cfgs, strategy, err := file.Runtime()
	if err != nil {
		return nil, []string{"backends config: " + err.Error()}
	}
	if len(cfgs) == 0 {
		return nil, nil
	}
	build := s.RegistryBuilder
	if build == nil {
		build = register.New
	}
	registry, err := build(cfgs, strategy)
	if err != nil {
		return nil, []string{"backends config: " + err.Error()}
	}
	found, errs := registry.Search(ctx, backend.Query{Unresolved: unresolved, Context: signals, Limit: discoveryResultLimit})
	warnings := make([]string, 0, len(errs))
	for _, err := range errs {
		warnings = append(warnings, "backend discovery: "+err.Error())
	}
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
		})
	}
	return out, warnings
}

// PlanInstall restores resolver order after UI selection. A nil selection picks suggested locals.
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
