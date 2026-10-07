package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go-s/internal/model"
)

func scanFixture(root string) model.ScanResult {
	return model.ScanResult{
		Root:         root,
		Members:      []string{"apps/a"},
		Observations: []model.Observation{{Key: "file:x", Kind: model.ObsConfig, Value: "x"}},
		Signals: []model.MergedSignal{{
			Key: "node:react", Domain: "node", Confidence: 0.9,
			Reasons: []string{"declared dependency"}, Evidence: []string{"package.json"}, Members: []string{"."},
		}},
		ResolveResult: model.ResolveResult{
			Suggestions: []model.Suggestion{
				{Skill: model.SkillRef{Source: "acme/react-skills", Name: "react"}, Bucket: "suggested", Confidence: 0.8, Reasons: []string{"technology node:react"}, Evidence: []string{"package.json"}},
				{Skill: model.SkillRef{Source: "https://skills.sh/x/y/react", Name: "react"}, Bucket: "external", ExternalScore: 0.75, SourceBackend: "skyll", URL: "https://skills.sh/x/y/react", Reasons: []string{"x/y"}},
			},
			Unresolved: []model.Observation{{Key: "pkg:npm:express", Kind: model.ObsPackage, Value: "express", Member: "."}},
		},
		Warnings: []string{"backend discovery: boom"},
	}
}

func TestScanEnvelopeCompact(t *testing.T) {
	root := t.TempDir()
	env := BuildScan(scanFixture(root), ScanOptions{})
	if env.SchemaVersion != ScanSchemaVersion || env.Kind != ScanKind {
		t.Fatalf("envelope identity = %+v", env)
	}
	if env.Summary.Suggested != 1 || env.Summary.External != 1 || env.Summary.Total != 2 || env.Summary.Signals != 1 || env.Summary.Unresolved != 1 || env.Summary.Warnings != 1 {
		t.Fatalf("summary = %+v", env.Summary)
	}
	if len(env.Observations) != 0 {
		t.Fatalf("compact must omit observations: %+v", env.Observations)
	}
	if len(env.Signals[0].Evidence) != 0 {
		t.Fatalf("compact must trim signal detail: %+v", env.Signals[0])
	}
	if len(env.Suggestions) != 2 {
		t.Fatalf("suggestions = %+v", env.Suggestions)
	}
	local, external := env.Suggestions[0], env.Suggestions[1]
	if local.ScoreType != "confidence" || local.Score != 0.8 || local.SourceKind != "github" || !local.Installable || local.InstallCommand == "" {
		t.Fatalf("local suggestion = %+v", local)
	}
	if !strings.Contains(local.InstallCommand, "npx") || !strings.Contains(local.InstallCommand, "acme/react-skills") {
		t.Fatalf("install command = %q", local.InstallCommand)
	}
	if external.ScoreType != "external" || external.Score != 0.75 || external.SourceKind != "url" || external.Backend != "skyll" {
		t.Fatalf("external suggestion = %+v", external)
	}
	if env.Install == nil || len(env.Install.Batches) != 1 || env.Install.Batches[0].Source != "acme/react-skills" {
		t.Fatalf("install plan = %+v", env.Install)
	}
	if len(env.Warnings) != 1 || env.Warnings[0].Source != "discovery" || env.Warnings[0].Message != "boom" {
		t.Fatalf("warnings = %+v", env.Warnings)
	}
}

func TestScanEnvelopeVerboseRestoresDetail(t *testing.T) {
	env := BuildScan(scanFixture(t.TempDir()), ScanOptions{Verbose: true})
	if len(env.Observations) != 1 {
		t.Fatalf("verbose observations = %+v", env.Observations)
	}
	if len(env.Signals[0].Evidence) != 1 || len(env.Signals[0].Reasons) != 1 {
		t.Fatalf("verbose signal = %+v", env.Signals[0])
	}
	if len(env.Suggestions[0].Reasons) == 0 || env.Suggestions[0].Confidence != 0.8 {
		t.Fatalf("verbose suggestion = %+v", env.Suggestions[0])
	}
}

func TestScanEnvelopeEmptyArraysAreNotNull(t *testing.T) {
	var out bytes.Buffer
	if err := ScanJSON(&out, model.ScanResult{}, ScanOptions{}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"members", "signals", "suggestions", "unresolved", "warnings"} {
		raw, ok := decoded[key]
		if !ok {
			t.Fatalf("missing key %q: %s", key, out.String())
		}
		if strings.TrimSpace(string(raw)) != "[]" {
			t.Fatalf("%s = %s, want []", key, raw)
		}
	}
	if _, ok := decoded["observations"]; ok {
		t.Fatalf("compact envelope must not include observations: %s", out.String())
	}
}

func TestScanEnvelopeUninstallableSource(t *testing.T) {
	result := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{{
		Skill: model.SkillRef{Source: "not a source!", Name: "x"}, Bucket: "external", ExternalScore: 0.5,
	}}}}
	env := BuildScan(result, ScanOptions{})
	got := env.Suggestions[0]
	if got.Installable || got.InstallCommand != "" {
		t.Fatalf("invalid source marked installable: %+v", got)
	}
	if got.SourceKind != "unknown" {
		t.Fatalf("sourceKind = %q", got.SourceKind)
	}
}
