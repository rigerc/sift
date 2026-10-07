package detect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigerc/sift/internal/rules"
	"github.com/rigerc/sift/internal/walk"
)

func TestEmbeddedCatalogRepresentativeFrameworkCoverage(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"package.json":         `{"dependencies":{"react":"1","next":"1","@nestjs/core":"1","fastify":"1","hono":"1","@playwright/test":"1","@prisma/client":"1","tailwindcss":"1","firebase":"1","@supabase/supabase-js":"1","phaser":"1"}}`,
		"pyproject.toml":       "[project]\ndependencies=[\"Django>=5\",\"fastapi[standard]>=1\",\"torch>=2\",\"scikit_learn>=1\",\"langchain-openai>=1\"]\n[tool.poetry.group.worker.dependencies]\ncelery=\"*\"\n",
		"go.mod":               "module example.com/demo\n\nrequire (\n github.com/gin-gonic/gin v1\n github.com/labstack/echo/v4 v1\n gorm.io/gorm v1\n)\n",
		"requirements-dev.txt": "pytest>=8\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := rules.Load("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Run(context.Background(), manifest, catalog.DetectionRules)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, signal := range got.Signals {
		seen[signal.Key] = true
	}
	for _, want := range []string{"node:react", "node:nextjs", "node:nestjs", "node:fastify", "node:hono", "node:playwright", "node:prisma", "node:tailwindcss", "node:firebase", "node:supabase", "node:phaser", "python:django", "python:fastapi", "python:pytorch", "python:scikit-learn", "python:langchain", "python:pytest", "python:celery", "go:gin", "go:echo", "go:gorm"} {
		if !seen[want] {
			t.Errorf("missing signal %q", want)
		}
	}
}

func TestPackageRulesStayInsideTheirEcosystem(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"package.json": `{"dependencies":{"django":"1","rails":"1"}}`, "requirements.txt": "react\n", "Gemfile": "gem \"express\"\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := walk.Run(context.Background(), root, walk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := rules.Load("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Run(context.Background(), w, catalog.DetectionRules)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"python:django": true, "ruby:rails": true, "node:react": true, "node:express": true}
	for _, signal := range got.Signals {
		if forbidden[signal.Key] {
			t.Errorf("cross-ecosystem false positive: %+v", signal)
		}
	}
}
