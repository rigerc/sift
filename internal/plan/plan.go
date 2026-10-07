// Package plan builds advisory commands for the upstream skills CLI.
// It never executes commands or reads or writes installation state.
package plan

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"go-s/internal/model"
)

// Package is the upstream skills CLI, installed on demand by npx. It is
// intentionally unpinned so the registry's latest published CLI is used.
const Package = "skills"

var (
	Agents     = []string{"claude-code", "opencode", "github-copilot", "codex"}
	repository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	skillSafe  = regexp.MustCompile(`[^a-z0-9._]+`)
)

func canonicalSource(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/")
	if u, err := url.Parse(s); err == nil && strings.EqualFold(u.Scheme, "https") && strings.EqualFold(u.Hostname(), "github.com") && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		p := strings.Trim(u.Path, "/")
		parts := strings.Split(p, "/")
		if len(parts) == 2 && repository.MatchString(p) && !strings.HasPrefix(p, ".") && !strings.Contains(p, "/.") {
			return strings.ToLower(p)
		}
	}
	if repository.MatchString(s) && !strings.HasPrefix(s, ".") && !strings.Contains(s, "/.") {
		return strings.ToLower(s)
	}
	return s
}

func canonicalSkillName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = skillSafe.ReplaceAllString(s, "-")
	s = strings.Trim(s, ".-")
	if len(s) > 255 {
		s = s[:255]
	}
	return s
}

// SourceKind classifies how a skill source is addressed, for machine-readable
// reporting. It never validates a source; callers should pair it with
// ValidateRef for structural checks. Neither classification nor structural
// validation establishes a source's trustworthiness.
func SourceKind(source string) string {
	s := strings.TrimSpace(source)
	if repository.MatchString(s) && !strings.HasPrefix(s, ".") && !strings.Contains(s, "/.") {
		return "github"
	}
	if u, err := url.Parse(s); err == nil && strings.EqualFold(u.Scheme, "https") && u.Hostname() != "" {
		if strings.EqualFold(u.Hostname(), "github.com") {
			return "github"
		}
		return "url"
	}
	if filepath.IsAbs(s) || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return "local"
	}
	return "unknown"
}

type Options struct {
	Agents     []string `json:"agents"`
	Global     bool     `json:"global"`
	AllowLocal bool     `json:"-"`
}
type Batch struct {
	Source string   `json:"source"`
	Skills []string `json:"skills"`
	Argv   []string `json:"argv"`
}
type Plan struct {
	Root    string   `json:"root"`
	Agents  []string `json:"agents"`
	Scope   string   `json:"scope"`
	Batches []Batch  `json:"batches"`
}

func safeValue(s string, limit int) bool {
	if s == "" || len(s) > limit || strings.HasPrefix(s, "-") {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

// ValidateRef checks reference structure, not whether its content is trusted
// or available. Check raw input before canonicalizing: trimming must never
// hide a control character or flag-shaped source.
func ValidateRef(ref model.SkillRef, allowLocal bool) error {
	if !safeValue(ref.Name, 200) || ref.Name == "." || ref.Name == ".." || ref.Name == "*" {
		return fmt.Errorf("invalid skill name %q", ref.Name)
	}
	if strings.ContainsAny(ref.Name, `/\`) {
		return fmt.Errorf("invalid skill name %q", ref.Name)
	}
	s := ref.Source
	if !safeValue(s, 2048) || strings.TrimSpace(s) != s {
		return fmt.Errorf("invalid skill source")
	}
	s = canonicalSource(s)
	if !safeValue(s, 2048) {
		return fmt.Errorf("invalid canonical skill source")
	}
	if repository.MatchString(s) && !strings.HasPrefix(s, ".") && !strings.Contains(s, "/.") {
		return nil
	}
	if strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err == nil && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path != "" && !strings.ContainsFunc(u.Path, unicode.IsControl) {
			return nil
		}
	}
	if allowLocal && (filepath.IsAbs(s) || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")) {
		return nil
	}
	return fmt.Errorf("unsupported skill source")
}

// Build preserves reference order, batching adjacent sources and dropping
// duplicate canonical identities. Distinct names must not share a destination.
// Its only filesystem observation is agent-directory detection when agents
// are omitted; no executable, skill contents, or installation state is consulted.
func Build(root string, refs []model.SkillRef, opts Options) (Plan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, err
	}
	agents := slices.Clone(opts.Agents)
	if len(agents) == 0 {
		agents = DetectAgents(root)
	}
	slices.Sort(agents)
	agents = slices.Compact(agents)
	for _, a := range agents {
		if !slices.Contains(Agents, a) {
			return Plan{}, fmt.Errorf("unsupported agent %q", a)
		}
	}
	p := Plan{Root: root, Agents: agents, Scope: "project", Batches: []Batch{}}
	if opts.Global {
		p.Scope = "global"
	}
	seen := map[string]bool{}
	names := map[string]model.SkillRef{}
	for _, ref := range refs {
		if err := ValidateRef(ref, opts.AllowLocal); err != nil {
			return Plan{}, err
		}
		ref.Source = canonicalSource(ref.Source)
		dest := canonicalSkillName(ref.Name)
		if dest == "" {
			return Plan{}, fmt.Errorf("invalid skill name %q", ref.Name)
		}
		if previous, ok := names[dest]; ok && previous != ref {
			return Plan{}, fmt.Errorf("destination collision for skill %q between %q (%s) and %q (%s)", dest, previous.Name, previous.Source, ref.Name, ref.Source)
		}
		names[dest] = ref
		if seen[ref.Key()] {
			continue
		}
		seen[ref.Key()] = true
		n := len(p.Batches)
		if n == 0 || p.Batches[n-1].Source != ref.Source {
			p.Batches = append(p.Batches, Batch{Source: ref.Source})
			n++
		}
		p.Batches[n-1].Skills = append(p.Batches[n-1].Skills, ref.Name)
	}
	for i := range p.Batches {
		p.Batches[i].Argv = AddArgs(p.Batches[i], agents, opts.Global)
	}
	return p, nil
}

func AddArgs(b Batch, agents []string, global bool) []string {
	args := []string{"--yes", Package, "add", b.Source, "--skill"}
	args = append(args, b.Skills...)
	args = append(args, "--agent")
	args = append(args, agents...)
	args = append(args, "--yes")
	if global {
		args = append(args, "--global")
	}
	return args
}

func DetectAgents(root string) []string {
	out := []string{}
	for dir, agent := range map[string]string{".claude": "claude-code", ".opencode": "opencode", ".github": "github-copilot", ".codex": "codex"} {
		if fi, err := os.Stat(filepath.Join(root, dir)); err == nil && fi.IsDir() {
			out = append(out, agent)
		}
	}
	if len(out) == 0 {
		out = append(out, "claude-code")
	}
	slices.Sort(out)
	return out
}
