// Package detect turns a cached walk into bounded, deterministic observations.
package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/walk"

	"github.com/pelletier/go-toml/v2"
)

type Result struct {
	Observations []model.Observation
	Signals      []model.Signal
	Unresolved   []model.Observation
	Warnings     []string
}

func Run(ctx context.Context, in walk.Result, rules []model.DetectionRule) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	out := Result{Warnings: append([]string(nil), in.Warnings...)}
	// Fixed layer slots make output independent of goroutine scheduling. Readers are
	// bounded by the walk's manifest; no detector traverses the filesystem again.
	var wave1 []model.Observation
	var layers [3][]model.Observation
	var wg sync.WaitGroup
	for slot := range layers {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			for _, member := range in.Members {
				if ctx.Err() != nil {
					return
				}
				switch slot {
				case 0:
					layers[slot] = append(layers[slot], manifestObs(in, member, rules)...)
				case 1:
					layers[slot] = append(layers[slot], fileObs(in, member, rules)...)
				case 2:
					layers[slot] = append(layers[slot], contextObs(member)...)
				}
			}
		}(slot)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	for _, layer := range layers {
		wave1 = append(wave1, layer...)
	}
	wave1 = normalize(wave1)
	out.Observations = append(out.Observations, wave1...)
	sig, unresolved := match(wave1, rules)
	out.Signals = append(out.Signals, sig...)
	// Explicit content rules corroborate matched technologies. Generic fallback
	// probes are limited to unresolved package/config targets.
	var wave2 []model.Observation
	for _, member := range in.Members {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		wave2 = append(wave2, contentObs(in, member, rules, unresolved)...)
	}
	wave2 = normalize(wave2)
	out.Observations = append(out.Observations, wave2...)
	sig2, unresolved2 := match(wave2, rules)
	out.Signals = append(out.Signals, sig2...)
	all := append(append([]model.Observation{}, unresolved...), unresolved2...)
	out.Unresolved = normalizeUnresolved(all)
	sort.Slice(out.Signals, func(i, j int) bool { return signalLess(out.Signals[i], out.Signals[j]) })
	return out, nil
}

func manifestObs(in walk.Result, m walk.Member, rules []model.DetectionRule) []model.Observation {
	var out []model.Observation
	for _, f := range m.Files {
		base := filepath.Base(f.Path)
		if !manifestCandidate(base, rules) {
			continue
		}
		// The manifest file itself is a technology signal for rules that
		// declare it (go:module, python:project, ...).
		if manifestRuleTarget(base, rules) {
			out = append(out, model.Observation{Key: "manifest:" + base, Kind: model.ObsManifest, Domain: manifestDomain(base), Value: base, Reason: "build manifest", Evidence: []string{f.Path}, Layer: 2, Member: m.Root})
		}
		b, err := readBounded(in.Root, f.Path, 1<<20)
		if err != nil {
			continue
		}
		s := string(b)
		for _, p := range packages(base, s) {
			domain := manifestDomain(base)
			out = append(out, model.Observation{Key: "pkg:" + domain + ":" + p, Kind: model.ObsPackage, Domain: domain, Value: p, Reason: "declared dependency", Evidence: []string{f.Path}, Layer: 2, Member: m.Root})
		}
	}
	return out
}

// manifestRuleTarget reports whether any detection rule declares base as a
// manifest signal. Manifests used only for package extraction (package.json)
// stay out of the observation stream.
func manifestRuleTarget(base string, rules []model.DetectionRule) bool {
	for _, r := range rules {
		for _, x := range r.Detect.Manifests {
			if x == base {
				return true
			}
		}
	}
	return false
}

func manifestCandidate(base string, rules []model.DetectionRule) bool {
	if isRequirementsFile(base) {
		return true
	}
	switch base {
	case "package.json", "composer.json", "pyproject.toml", "Cargo.toml", "go.mod", "Gemfile", "Pipfile":
		return true
	}
	for _, r := range rules {
		for _, x := range r.Detect.Manifests {
			if x == base {
				return true
			}
		}
	}
	return false
}

func manifestDomain(base string) string {
	if isRequirementsFile(base) {
		return "python"
	}
	switch base {
	case "package.json":
		return "npm"
	case "composer.json":
		return "composer"
	case "pyproject.toml", "Pipfile":
		return "python"
	case "Cargo.toml":
		return "cargo"
	case "go.mod":
		return "go"
	case "Gemfile":
		return "ruby"
	}
	return "unknown"
}

func packages(base, s string) []string {
	if isRequirementsFile(base) {
		return parseRequirements(s)
	}
	switch base {
	case "package.json":
		var v struct {
			Dependencies map[string]any `json:"dependencies"`
			Dev          map[string]any `json:"devDependencies"`
			Peer         map[string]any `json:"peerDependencies"`
			Optional     map[string]any `json:"optionalDependencies"`
		}
		if json.Unmarshal([]byte(s), &v) != nil {
			return nil
		}
		set := map[string]any{}
		for _, m := range []map[string]any{v.Dependencies, v.Dev, v.Peer, v.Optional} {
			for k := range m {
				set[k] = true
			}
		}
		return sortedKeys(set)
	case "composer.json":
		return uniqSorted(append(jsonMapKeys(s, "require"), jsonMapKeys(s, "require-dev")...))
	case "go.mod":
		var o []string
		inBlock := false
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			switch {
			case fields[0] == "require" && len(fields) >= 2 && strings.HasPrefix(fields[1], "("):
				inBlock = true
			case fields[0] == "require" && len(fields) >= 2:
				o = append(o, fields[1])
			case fields[0] == ")":
				inBlock = false
			case inBlock:
				o = append(o, fields[0])
			}
		}
		return uniqSorted(o)
	case "Gemfile":
		var o []string
		re := regexp.MustCompile(`(?m)^\s*gem\s+["']([^"']+)["']`)
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			o = append(o, m[1])
		}
		return uniqSorted(o)
	case "pyproject.toml":
		return parseTOMLDependencies(s, "python")
	case "Pipfile":
		return parsePipfileDependencies(s)
	case "Cargo.toml":
		return parseTOMLDependencies(s, "cargo")
	default:
		return nil
	}
}

func isRequirementsFile(base string) bool {
	lower := strings.ToLower(base)
	return strings.HasPrefix(lower, "requirements") && strings.HasSuffix(lower, ".txt")
}

func parseRequirements(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if name := pythonRequirementName(line); name != "" {
			out = append(out, name)
		}
	}
	return uniqSorted(out)
}

func parsePipfileDependencies(s string) []string {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(s), &doc); err != nil {
		return nil
	}
	var out []string
	for _, section := range []string{"packages", "dev-packages"} {
		if value, ok := doc[section]; ok {
			collectDependencyContainer(value, "python", &out)
		}
	}
	return uniqSorted(out)
}

func parseTOMLDependencies(s, domain string) []string {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(s), &doc); err != nil {
		return nil
	}
	var out []string
	var visit func(map[string]any)
	visit = func(m map[string]any) {
		for key, value := range m {
			lower := strings.ToLower(key)
			if lower == "dependencies" || strings.HasSuffix(lower, "-dependencies") || lower == "dependency-groups" {
				collectDependencyContainer(value, domain, &out)
			}
			if child, ok := value.(map[string]any); ok {
				visit(child)
			}
		}
	}
	visit(doc)
	return uniqSorted(out)
}

func collectDependencyContainer(value any, domain string, out *[]string) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if raw, ok := item.(string); ok {
				if name := dependencyStringName(raw, domain); name != "" {
					*out = append(*out, name)
				}
			}
		}
	case map[string]any:
		for key, item := range v {
			if domain == "python" && strings.EqualFold(key, "python") {
				continue
			}
			if table, ok := item.(map[string]any); ok {
				if actual, ok := table["package"].(string); ok && actual != "" {
					*out = append(*out, normalizeDependencyName(actual, domain))
					continue
				}
			}
			*out = append(*out, normalizeDependencyName(key, domain))
		}
	}
}

func dependencyStringName(raw, domain string) string {
	if domain == "python" {
		return pythonRequirementName(raw)
	}
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return ""
	}
	return normalizeDependencyName(fields[0], domain)
}

var (
	pythonNameRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*`)
	pythonNormalizeRE = regexp.MustCompile(`[-_.]+`)
)

func pythonRequirementName(raw string) string {
	m := pythonNameRE.FindString(strings.TrimSpace(raw))
	if m == "" {
		return ""
	}
	return normalizeDependencyName(m, "python")
}

func normalizeDependencyName(name, domain string) string {
	if domain == "python" {
		return pythonNormalizeRE.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	}
	return strings.TrimSpace(name)
}

func jsonMapKeys(s, key string) []string {
	var v map[string]json.RawMessage
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	var m map[string]any
	if x := v[key]; json.Unmarshal(x, &m) != nil {
		return nil
	}
	return sortedKeys(m)
}

func sortedKeys(m map[string]any) []string {
	o := make([]string, 0, len(m))
	for k := range m {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}

func uniqSorted(a []string) []string {
	m := map[string]any{}
	for _, x := range a {
		m[x] = nil
	}
	return sortedKeys(m)
}

func fileObs(in walk.Result, m walk.Member, rules []model.DetectionRule) []model.Observation {
	var out []model.Observation
	configured := map[string]bool{}
	for _, r := range rules {
		for _, fr := range r.Detect.ConfigFiles {
			configured[fr.Pattern] = true
			files := m.Files
			if fr.Mode != model.MatchGlob {
				files = nil
				for _, p := range in.NameIndex[fr.Pattern] {
					for _, f := range m.Files {
						if f.Path == p {
							files = append(files, f)
							break
						}
					}
				}
			}
			for _, f := range files {
				base := filepath.Base(f.Path)
				match := fr.Mode != model.MatchGlob && base == fr.Pattern
				if fr.Mode == model.MatchGlob {
					match, _ = filepath.Match(fr.Pattern, base)
				}
				if match {
					out = append(out, model.Observation{Key: "file:" + base, Kind: model.ObsConfig, Domain: "config", Value: base, Reason: "configuration file", Evidence: []string{f.Path}, Layer: 3, Member: m.Root})
				}
			}
		}
		for _, er := range r.Detect.FileExtensions {
			n := 0
			for _, f := range m.Files {
				if f.Ext == strings.ToLower(er.Extension) {
					n++
				}
			}
			if n >= max(1, er.MinCount) {
				out = append(out, model.Observation{Key: "ext:" + strings.ToLower(er.Extension), Kind: model.ObsExt, Domain: "files", Value: strings.ToLower(er.Extension), Reason: fmt.Sprintf("%d matching files", n), Evidence: extensionEvidence(m, er.Extension), Layer: 3, Member: m.Root})
			}
		}
		for _, d := range r.Detect.Directories {
			target := strings.Trim(filepath.ToSlash(d), "/")
			for _, f := range m.Files {
				relPath := memberRelativePath(m.Root, f.Path)
				dir := filepath.ToSlash(filepath.Dir(relPath))
				if dir == target || strings.HasPrefix(dir, target+"/") {
					out = append(out, model.Observation{Key: "dir:" + d, Kind: model.ObsConfig, Domain: "files", Value: d, Reason: "directory present", Evidence: []string{f.Path}, Layer: 3, Member: m.Root})
					break
				}
			}
		}
	}
	for _, f := range m.Files {
		base := filepath.Base(f.Path)
		if manifestRuleTarget(base, rules) {
			continue
		}
		lower := strings.ToLower(base)
		if !configured[base] && (strings.Contains(lower, "config") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")) {
			out = append(out, model.Observation{Key: "file:" + base, Kind: model.ObsConfig, Domain: "config", Value: base, Reason: "framework-shaped configuration file", Evidence: []string{f.Path}, Layer: 3, Member: m.Root})
		}
	}
	return out
}

func extensionEvidence(m walk.Member, e string) []string {
	var o []string
	for _, f := range m.Files {
		if f.Ext == strings.ToLower(e) {
			o = append(o, f.Path)
		}
	}
	sort.Strings(o)
	return o
}

func contextObs(m walk.Member) []model.Observation {
	var o []model.Observation
	for _, f := range m.Files {
		b := strings.ToLower(filepath.Base(f.Path))
		if b == "readme.md" || strings.HasPrefix(b, "readme.") {
			o = append(o, model.Observation{Key: "context:readme", Kind: model.ObsContext, Domain: "docs", Value: "readme", Reason: "README present", Evidence: []string{f.Path}, Layer: 5, Member: m.Root})
		}
		relPath := strings.ToLower(memberRelativePath(m.Root, f.Path))
		if strings.HasPrefix(relPath, ".github/workflows/") {
			o = append(o, model.Observation{Key: "context:ci", Kind: model.ObsContext, Domain: "ci", Value: "github-actions", Reason: "CI workflow present", Evidence: []string{f.Path}, Layer: 5, Member: m.Root})
		}
	}
	return o
}

func memberRelativePath(memberRoot, path string) string {
	path = filepath.ToSlash(path)
	root := strings.Trim(filepath.ToSlash(memberRoot), "/")
	if root == "" || root == "." {
		return path
	}
	prefix := root + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func contentObs(in walk.Result, m walk.Member, rules []model.DetectionRule, unresolved []model.Observation) []model.Observation {
	var o []model.Observation
	type cachedRead struct {
		limit int64
		data  []byte
		err   error
	}
	cache := map[string]cachedRead{}
	readCached := func(path string, limit int64) ([]byte, error) {
		if entry, ok := cache[path]; ok && entry.limit >= limit {
			if entry.err != nil {
				return nil, entry.err
			}
			if int64(len(entry.data)) > limit {
				return entry.data[:int(limit)], nil
			}
			return entry.data, nil
		}
		b, err := readBounded(in.Root, path, limit)
		cache[path] = cachedRead{limit: limit, data: b, err: err}
		return b, err
	}
	for _, r := range rules {
		for _, cr := range r.Detect.Content {
			for _, f := range m.Files {
				base := filepath.Base(f.Path)
				ok, _ := filepath.Match(cr.FilePattern, base)
				if !ok && base != cr.FilePattern {
					continue
				}
				limit := cr.ReadLimit
				if limit <= 0 {
					limit = 4096
				}
				if limit > 65536 {
					limit = 65536
				}
				b, e := readCached(f.Path, limit)
				if e != nil {
					continue
				}
				if bytesBinary(b) {
					continue
				}
				for _, p := range cr.Patterns {
					re, e := regexp.Compile(p)
					if e != nil {
						continue
					}
					if re.Match(b) {
						o = append(o, model.Observation{Key: "content:" + p, Kind: model.ObsContent, Domain: "content", Value: p, Reason: "content pattern matched", Evidence: []string{f.Path}, Layer: 4, Member: m.Root})
						break
					}
				}
			}
		}
	}
	// Generic probes are restricted to unresolved framework-shaped config files.
	// They provide stable context for later discovery without reading the whole
	// workspace or turning arbitrary extensions into discovery triggers.
	for _, u := range unresolved {
		if u.Member != m.Root || u.Kind != model.ObsConfig {
			continue
		}
		for _, p := range u.Evidence {
			b, e := readCached(p, 4096)
			if e != nil || bytesBinary(b) || len(strings.TrimSpace(string(b))) == 0 {
				continue
			}
			base := filepath.Base(p)
			o = append(o, model.Observation{Key: "content:file:" + base, Kind: model.ObsContent, Domain: "content", Value: base, Reason: "bounded generic content probe", Evidence: []string{p}, Layer: 4, Member: m.Root})
		}
	}
	return o
}

func bytesBinary(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	return strings.IndexByte(string(b[:n]), 0) >= 0
}

func readBounded(root, name string, limit int64) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(name))
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return nil, err
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return nil, err
	}
	if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes workspace")
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if limit > 0 {
		return io.ReadAll(io.LimitReader(f, limit))
	}
	return io.ReadAll(f)
}

func match(obs []model.Observation, rules []model.DetectionRule) ([]model.Signal, []model.Observation) {
	var s []model.Signal
	var u []model.Observation
	for _, o := range obs {
		matched := false
		for _, r := range rules {
			if ruleMatches(o, r) {
				matched = true
				s = append(s, model.Signal{Key: r.TechnologyID, Domain: o.Domain, Reason: o.Reason, Evidence: o.Evidence, Layer: o.Layer, Member: o.Member, Kind: o.Kind})
			}
		}
		if !matched && o.Kind != model.ObsContext && o.Kind != model.ObsExt {
			u = append(u, o)
		}
	}
	return s, u
}

func ruleMatches(o model.Observation, r model.DetectionRule) bool {
	d := r.Detect
	switch o.Kind {
	case model.ObsPackage:
		if expected := packageDomainForTechnology(r.TechnologyID); expected != "" && o.Domain != expected {
			return false
		}
		for _, x := range d.Packages {
			if x == o.Value {
				return true
			}
		}
		for _, x := range d.PackagePatterns {
			if ok, _ := filepath.Match(x, o.Value); ok {
				return true
			}
		}
	case model.ObsConfig:
		for _, x := range d.ConfigFiles {
			if x.Mode == model.MatchGlob {
				if ok, _ := filepath.Match(x.Pattern, o.Value); ok {
					return true
				}
			} else if x.Pattern == o.Value {
				return true
			}
		}
		for _, x := range d.Directories {
			if x == o.Value {
				return true
			}
		}
	case model.ObsManifest:
		for _, x := range d.Manifests {
			if x == o.Value {
				return true
			}
		}
	case model.ObsExt:
		for _, x := range d.FileExtensions {
			if strings.EqualFold(x.Extension, o.Value) && len(o.Evidence) >= max(1, x.MinCount) {
				return true
			}
		}
	case model.ObsContent:
		for _, x := range d.Content {
			for _, p := range x.Patterns {
				if p == o.Value {
					return true
				}
			}
		}
	}
	return false
}

func packageDomainForTechnology(technologyID string) string {
	prefix, _, _ := strings.Cut(technologyID, ":")
	switch prefix {
	case "node":
		return "npm"
	case "python":
		return "python"
	case "go":
		return "go"
	case "rust":
		return "cargo"
	case "php":
		return "composer"
	case "ruby":
		return "ruby"
	default:
		return ""
	}
}

func normalize(a []model.Observation) []model.Observation {
	sort.Slice(a, func(i, j int) bool { return obsLess(a[i], a[j]) })
	o := a[:0]
	for _, x := range a {
		if len(o) > 0 && obsKey(o[len(o)-1]) == obsKey(x) {
			o[len(o)-1].Evidence = union(o[len(o)-1].Evidence, x.Evidence)
			continue
		}
		x.Evidence = union(nil, x.Evidence)
		o = append(o, x)
	}
	return o
}

func normalizeUnresolved(a []model.Observation) []model.Observation {
	m := map[string]model.Observation{}
	for _, x := range a {
		m[obsKey(x)] = x
	}
	o := make([]model.Observation, 0, len(m))
	for _, x := range m {
		o = append(o, x)
	}
	return normalize(o)
}

func obsKey(x model.Observation) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s", x.Kind, x.Key, x.Member, strings.Join(x.Evidence, "\x00"))
}
func obsLess(a, b model.Observation) bool { return obsKey(a) < obsKey(b) }
func signalLess(a, b model.Signal) bool {
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	if a.Member != b.Member {
		return a.Member < b.Member
	}
	return a.Layer < b.Layer
}

func union(a, b []string) []string {
	m := map[string]bool{}
	for _, x := range append(a, b...) {
		m[x] = true
	}
	o := make([]string, 0, len(m))
	for x := range m {
		o = append(o, x)
	}
	sort.Strings(o)
	return o
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
