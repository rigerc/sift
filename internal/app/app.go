// Package app provides UI-independent application operations.
package app

import (
	"context"
	"fmt"
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
	slices.Sort(warnings)
	warnings = slices.Compact(warnings)
	return model.ScanResult{Root: manifest.Root, Members: members, Observations: detection.Observations, Signals: signals, ResolveResult: resolved, Warnings: warnings}, nil
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
