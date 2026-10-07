package rules

import (
	"go-s/internal/model"
	"testing"
)

func TestEmbeddedCatalogBreadthAndRepresentativeIDs(t *testing.T) {
	catalog, err := Load("")
	if err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	if len(catalog.Technologies) < 240 {
		t.Fatalf("catalog unexpectedly narrow: %d", len(catalog.Technologies))
	}
	if len(catalog.DetectionRules) != len(catalog.Technologies) {
		t.Fatalf("technologies=%d detection=%d", len(catalog.Technologies), len(catalog.DetectionRules))
	}
	for _, id := range []string{"node:nextjs", "node:nuxt", "node:sveltekit", "node:nestjs", "node:hono", "python:django", "python:fastapi", "python:pytorch", "python:langchain", "go:gin", "go:fiber", "rust:axum", "rust:leptos", "php:laravel", "ruby:rails", "java:spring-boot", "kotlin:ktor", "dotnet:aspnetcore", "elixir:phoenix", "dart:flutter", "infra:kubernetes", "infra:helm", "ci:github-actions", "game:unity", "game:godot"} {
		if _, ok := catalog.Technology(id); !ok {
			t.Errorf("missing technology %q", id)
		}
		if _, ok := catalog.Detection(id); !ok {
			t.Errorf("missing detection rule %q", id)
		}
	}
}

func TestValidateDetectRejectsMalformedCatalogPatterns(t *testing.T) {
	for _, detect := range []model.DetectConfig{{PackagePatterns: []string{"["}}, {ConfigFiles: []model.FileRule{{Pattern: "[", Mode: model.MatchGlob}}}, {Content: []model.ContentRule{{FilePattern: "[", Patterns: []string{"ok"}}}}, {Directories: []string{"../outside"}}} {
		if err := validateDetect(detect); err == nil {
			t.Error("expected validation error")
		}
	}
}
