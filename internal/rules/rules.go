// Package rules loads the embedded technology and skill catalogs and validates
// optional local overrides.
package rules

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"go-s/internal/model"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Catalog is an immutable, validated rule set. Its slices are kept public so
// callers can render and hash the exact catalog used for a scan.
type Catalog struct {
	Technologies   []model.Technology
	DetectionRules []model.DetectionRule
	SkillRules     []model.SkillRule
	ComboRules     []model.ComboRule

	technologyByID map[string]model.Technology
	detectionByID  map[string]model.DetectionRule
	skillByID      map[string]model.SkillRule
}

//go:embed data/*.yaml
var embedded embed.FS

type fileCatalog struct {
	Technologies   []model.Technology    `yaml:"technologies"`
	DetectionRules []model.DetectionRule `yaml:"detection_rules"`
	SkillRules     []model.SkillRule     `yaml:"skill_rules"`
	ComboRules     []model.ComboRule     `yaml:"combos"`
}

// Decode validates a complete catalog document without reading a file.
func Decode(data []byte) (Catalog, error) {
	var raw fileCatalog
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog: %w", err)
	}
	return validate(raw)
}

// Merge overlays replacement entries from override onto base by stable IDs.
func Merge(base, override Catalog) (Catalog, error) {
	raw := mergeFileCatalog(fileCatalog{Technologies: base.Technologies, DetectionRules: base.DetectionRules, SkillRules: base.SkillRules, ComboRules: base.ComboRules}, fileCatalog{Technologies: override.Technologies, DetectionRules: override.DetectionRules, SkillRules: override.SkillRules, ComboRules: override.ComboRules})
	return validate(raw)
}

// Hash returns a stable content hash for the canonical catalog representation.
func Hash(c Catalog) string {
	data, _ := yaml.Marshal(fileCatalog{Technologies: c.Technologies, DetectionRules: c.DetectionRules, SkillRules: c.SkillRules, ComboRules: c.ComboRules})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

// Load loads embedded rules, then replaces entries with the same stable ID
// from path. An empty path means embedded rules only.
func Load(path string) (Catalog, error) {
	var out fileCatalog
	for _, name := range []string{"technologies.yaml", "detection.yaml", "skills.yaml", "combos.yaml"} {
		data, err := embedded.ReadFile("data/" + name)
		if err != nil {
			return Catalog{}, fmt.Errorf("read embedded %s: %w", name, err)
		}
		var part fileCatalog
		if err := yaml.Unmarshal(data, &part); err != nil {
			return Catalog{}, fmt.Errorf("parse embedded %s: %w", name, err)
		}
		out = mergeFileCatalog(out, part)
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("read catalog override %q: %w", path, err)
		}
		var override fileCatalog
		if err := yaml.Unmarshal(data, &override); err != nil {
			return Catalog{}, fmt.Errorf("parse catalog override %q: %w", path, err)
		}
		out = mergeFileCatalog(out, override)
	}
	return validate(out)
}

func mergeFileCatalog(base, add fileCatalog) fileCatalog {
	base.Technologies = replaceBy(base.Technologies, add.Technologies, func(v model.Technology) string { return v.ID })
	base.DetectionRules = replaceBy(base.DetectionRules, add.DetectionRules, func(v model.DetectionRule) string {
		if v.ID != "" {
			return v.ID
		}
		return v.TechnologyID
	})
	base.SkillRules = replaceBy(base.SkillRules, add.SkillRules, func(v model.SkillRule) string {
		if v.ID != "" {
			return v.ID
		}
		return v.TechnologyID
	})
	base.ComboRules = replaceBy(base.ComboRules, add.ComboRules, func(v model.ComboRule) string { return v.ID })
	return base
}

func replaceBy[T any](base, add []T, key func(T) string) []T {
	result := append([]T(nil), base...)
	positions := make(map[string]int, len(result))
	for i, item := range result {
		if id := key(item); id != "" {
			positions[id] = i
		}
	}
	for _, item := range add {
		id := key(item)
		if i, ok := positions[id]; ok && id != "" {
			result[i] = item
		} else {
			positions[id] = len(result)
			result = append(result, item)
		}
	}
	return result
}

var (
	technologyIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?::[a-z0-9][a-z0-9._/-]*)?$`)
	sourcePattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)+$`)
)

func validate(raw fileCatalog) (Catalog, error) {
	c := Catalog{
		Technologies: raw.Technologies, DetectionRules: raw.DetectionRules, SkillRules: raw.SkillRules, ComboRules: raw.ComboRules,
		technologyByID: make(map[string]model.Technology), detectionByID: make(map[string]model.DetectionRule), skillByID: make(map[string]model.SkillRule),
	}
	for _, technology := range c.Technologies {
		if !technologyIDPattern.MatchString(technology.ID) {
			return Catalog{}, fmt.Errorf("invalid technology id %q", technology.ID)
		}
		if _, exists := c.technologyByID[technology.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate technology id %q", technology.ID)
		}
		c.technologyByID[technology.ID] = technology
	}
	for i, rule := range c.DetectionRules {
		id := rule.ID
		if id == "" {
			id = rule.TechnologyID
		}
		if _, exists := c.detectionByID[id]; exists {
			return Catalog{}, fmt.Errorf("duplicate detection rule id %q", id)
		}
		if _, exists := c.technologyByID[rule.TechnologyID]; !exists {
			return Catalog{}, fmt.Errorf("detection rule %q references unknown technology %q", id, rule.TechnologyID)
		}
		if err := validateDetect(rule.Detect); err != nil {
			return Catalog{}, fmt.Errorf("detection rule %q: %w", id, err)
		}
		c.detectionByID[id] = c.DetectionRules[i]
	}
	for i, rule := range c.SkillRules {
		id := rule.ID
		if id == "" {
			id = rule.TechnologyID
		}
		if _, exists := c.skillByID[id]; exists {
			return Catalog{}, fmt.Errorf("duplicate skill rule id %q", id)
		}
		if _, exists := c.technologyByID[rule.TechnologyID]; !exists {
			return Catalog{}, fmt.Errorf("skill rule %q references unknown technology %q", id, rule.TechnologyID)
		}
		if rule.RuleConfidence == 0 {
			c.SkillRules[i].RuleConfidence = 1
			rule.RuleConfidence = 1
		}
		if rule.RuleConfidence < 0 || rule.RuleConfidence > 1 {
			return Catalog{}, fmt.Errorf("skill rule %q confidence must be between 0 and 1", id)
		}
		for _, skill := range rule.Skills {
			if err := validateSkill(skill); err != nil {
				return Catalog{}, fmt.Errorf("skill rule %q: %w", id, err)
			}
		}
		c.skillByID[id] = c.SkillRules[i]
	}
	for _, combo := range c.ComboRules {
		if combo.ID == "" {
			return Catalog{}, errors.New("combo rule has empty id")
		}
		for _, trigger := range append(append([]string{}, combo.Triggers...), combo.Conflicts...) {
			if _, ok := c.technologyByID[trigger]; !ok {
				return Catalog{}, fmt.Errorf("combo %q references unknown technology %q", combo.ID, trigger)
			}
		}
		for _, skill := range append(append([]model.SkillRef{}, combo.Skills...), combo.Order...) {
			if err := validateSkill(skill); err != nil {
				return Catalog{}, fmt.Errorf("combo %q: %w", combo.ID, err)
			}
		}
	}
	return c, nil
}

func validateDetect(d model.DetectConfig) error {
	for _, pattern := range d.PackagePatterns {
		if _, err := filepath.Match(pattern, "probe"); err != nil {
			return fmt.Errorf("invalid package glob %q: %w", pattern, err)
		}
	}
	for _, file := range d.ConfigFiles {
		if file.Pattern == "" {
			return errors.New("empty file pattern")
		}
		if file.Mode == "" {
			file.Mode = model.MatchExact
		}
		if file.Mode != model.MatchExact && file.Mode != model.MatchGlob {
			return fmt.Errorf("invalid file mode %q", file.Mode)
		}
		if file.Mode == model.MatchGlob {
			if _, err := filepath.Match(file.Pattern, "probe"); err != nil {
				return fmt.Errorf("invalid file glob %q: %w", file.Pattern, err)
			}
		}
	}
	for _, ext := range d.FileExtensions {
		if ext.Extension == "" || ext.MinCount < 1 {
			return fmt.Errorf("invalid extension rule %q", ext.Extension)
		}
	}
	for _, dir := range d.Directories {
		clean := filepath.ToSlash(filepath.Clean(dir))
		if strings.TrimSpace(dir) == "" || filepath.IsAbs(dir) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("invalid directory rule %q", dir)
		}
	}
	for _, content := range d.Content {
		if content.FilePattern == "" {
			return errors.New("empty content file pattern")
		}
		if _, err := filepath.Match(content.FilePattern, "probe"); err != nil {
			return fmt.Errorf("invalid content file glob %q: %w", content.FilePattern, err)
		}
		for _, pattern := range content.Patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("invalid content regex %q: %w", pattern, err)
			}
		}
	}
	return nil
}

func validateSkill(skill model.SkillRef) error {
	if err := validateText("skill source", skill.Source, 512); err != nil {
		return err
	}
	if err := validateText("skill name", skill.Name, 256); err != nil {
		return err
	}
	if strings.HasPrefix(skill.Source, "-") || strings.HasPrefix(skill.Name, "-") {
		return errors.New("skill source/name cannot start with '-'")
	}
	if strings.ContainsAny(skill.Source, "\\\n\r\t") {
		return errors.New("skill source contains control characters")
	}
	if u, err := url.Parse(skill.Source); err == nil && u.Scheme != "" {
		if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "git" && u.Scheme != "ssh" && u.Scheme != "file" {
			return fmt.Errorf("unsupported skill source scheme %q", u.Scheme)
		}
	} else if !sourcePattern.MatchString(strings.TrimSuffix(skill.Source, "/")) && !strings.HasPrefix(skill.Source, ".") && !strings.HasPrefix(skill.Source, "/") {
		return fmt.Errorf("invalid skill source %q", skill.Source)
	}
	return nil
}

func validateText(label, value string, max int) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > max || strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func (c Catalog) Technology(id string) (model.Technology, bool) {
	v, ok := c.technologyByID[id]
	return v, ok
}

func (c Catalog) Detection(id string) (model.DetectionRule, bool) {
	v, ok := c.detectionByID[id]
	return v, ok
}

func (c Catalog) SkillRule(id string) (model.SkillRule, bool) {
	for _, v := range c.SkillRules {
		if v.TechnologyID == id {
			return v, true
		}
	}
	return model.SkillRule{}, false
}

func (c Catalog) SortedTechnologyIDs() []string {
	out := make([]string, 0, len(c.Technologies))
	for _, v := range c.Technologies {
		out = append(out, v.ID)
	}
	sort.Strings(out)
	return out
}
