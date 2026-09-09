// Package model defines UI-independent scan and skill identities.
package model

import "strings"

type ObservationKind string

const (
	ObsPackage  ObservationKind = "package"
	ObsConfig   ObservationKind = "config"
	ObsExt      ObservationKind = "extension"
	ObsContent  ObservationKind = "content"
	ObsContext  ObservationKind = "context"
	ObsManifest ObservationKind = "manifest"
)

type Observation struct {
	Key      string          `json:"key"`
	Kind     ObservationKind `json:"kind"`
	Domain   string          `json:"domain"`
	Value    string          `json:"value"`
	Reason   string          `json:"reason"`
	Evidence []string        `json:"evidence"`
	Layer    int             `json:"layer"`
	Member   string          `json:"member"`
}
type Signal struct {
	Key      string          `json:"key"`
	Domain   string          `json:"domain"`
	Reason   string          `json:"reason"`
	Evidence []string        `json:"evidence"`
	Layer    int             `json:"layer"`
	Member   string          `json:"member"`
	Kind     ObservationKind `json:"kind,omitempty"`
}
type MergedSignal struct {
	Key          string   `json:"key"`
	Domain       string   `json:"domain"`
	Reasons      []string `json:"reasons"`
	Evidence     []string `json:"evidence"`
	Confidence   float64  `json:"confidence"`
	Members      []string `json:"members"`
	RootObserved bool     `json:"rootObserved"`
	Layers       []int    `json:"layers"`
}
type MatchMode string

const (
	MatchExact MatchMode = "exact"
	MatchGlob  MatchMode = "glob"
)

type FileRule struct {
	Pattern string    `json:"pattern" yaml:"pattern"`
	Mode    MatchMode `json:"mode" yaml:"mode"`
}
type ExtensionRule struct {
	Extension string `json:"extension" yaml:"extension"`
	MinCount  int    `json:"minCount" yaml:"min_count"`
}
type ContentRule struct {
	FilePattern string   `json:"filePattern" yaml:"file_pattern"`
	Patterns    []string `json:"patterns" yaml:"patterns"`
	ReadLimit   int64    `json:"readLimit" yaml:"read_limit"`
}
type DetectConfig struct {
	Packages        []string        `json:"packages" yaml:"packages"`
	PackagePatterns []string        `json:"packagePatterns" yaml:"package_patterns"`
	ConfigFiles     []FileRule      `json:"configFiles" yaml:"config_files"`
	FileExtensions  []ExtensionRule `json:"fileExtensions" yaml:"file_extensions"`
	Content         []ContentRule   `json:"content" yaml:"content"`
	Manifests       []string        `json:"manifests" yaml:"manifests"`
	Directories     []string        `json:"directories,omitempty" yaml:"directories"`
}
type Technology struct {
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name" yaml:"name"`
	Domain string `json:"domain" yaml:"domain"`
}
type DetectionRule struct {
	ID           string       `json:"id,omitempty" yaml:"id"`
	TechnologyID string       `json:"technologyId" yaml:"technology_id"`
	Detect       DetectConfig `json:"detect" yaml:"detect"`
}
type SkillRef struct {
	Source string `json:"source" yaml:"source"`
	Name   string `json:"name" yaml:"name"`
}

func (s SkillRef) Key() string {
	return strings.TrimSuffix(strings.TrimSpace(s.Source), "/") + "\x00" + s.Name
}

type SkillRule struct {
	ID             string     `json:"id,omitempty" yaml:"id"`
	TechnologyID   string     `json:"technologyId" yaml:"technology_id"`
	Skills         []SkillRef `json:"skills" yaml:"skills"`
	RuleConfidence float64    `json:"ruleConfidence" yaml:"rule_confidence"`
}
type ComboRule struct {
	ID        string     `json:"id" yaml:"id"`
	Triggers  []string   `json:"triggers" yaml:"triggers"`
	Skills    []SkillRef `json:"skills" yaml:"skills"`
	Order     []SkillRef `json:"order" yaml:"order"`
	Conflicts []string   `json:"conflicts" yaml:"conflicts"`
}
type Suggestion struct {
	Skill         SkillRef `json:"skill"`
	Confidence    float64  `json:"confidence"`
	Bucket        string   `json:"bucket"`
	Reasons       []string `json:"reasons"`
	Evidence      []string `json:"evidence"`
	Technologies  []string `json:"technologies"`
	Members       []string `json:"members"`
	ExternalScore float64  `json:"externalScore,omitempty"`
	SourceBackend string   `json:"sourceBackend,omitempty"`
	Stale         bool     `json:"stale,omitempty"`
}
type ResolveResult struct {
	Suggestions []Suggestion  `json:"suggestions"`
	Unresolved  []Observation `json:"unresolved"`
	Order       []SkillRef    `json:"order"`
}
type ScanResult struct {
	Root         string         `json:"root"`
	Members      []string       `json:"members"`
	Observations []Observation  `json:"observations"`
	Signals      []MergedSignal `json:"signals"`
	ResolveResult
	Warnings []string `json:"warnings"`
}
