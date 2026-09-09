// Package install plans and executes the pinned upstream skills CLI.
package install

import (
	"context"
	"fmt"
	"go-s/internal/model"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Verified against the npm skills@1.5.25 package metadata and dist/cli.mjs.
const (
	Package     = "skills@1.5.25"
	MinimumNode = "22.20.0"
)

var (
	Agents     = []string{"claude-code", "opencode", "github-copilot", "codex"}
	repository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	skillSafe  = regexp.MustCompile(`[^a-z0-9._]+`)
)

// Backend sources dispatch to their own pinned CLI instead of the upstream
// skills CLI. Empty backend means the GitHub-repo/URL/local skills path.
const (
	BackendAskill    = "askill"
	BackendSkillfish = "skillfish"
	BackendSmithery  = "smithery"
)

var backendBinaries = map[string]string{
	BackendAskill:    "askill",
	BackendSkillfish: "skillfish",
	BackendSmithery:  "smithery",
}

// BackendForSource reports which CLI manages a source; empty means the pinned
// upstream skills CLI handles it (GitHub repos, HTTPS URLs, local paths).
func BackendForSource(source string) string {
	switch s := canonicalSource(source); s {
	case BackendAskill, BackendSkillfish, BackendSmithery:
		return s
	}
	return ""
}

func canonicalSource(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/")
	if u, err := url.Parse(s); err == nil && strings.EqualFold(u.Scheme, "https") && strings.EqualFold(u.Hostname(), "github.com") && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		p := strings.Trim(u.Path, "/")
		parts := strings.Split(p, "/")
		if len(parts) == 2 {
			return strings.ToLower(parts[0]) + "/" + strings.ToLower(parts[1])
		}
	}
	if repository.MatchString(s) {
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

type Options struct {
	Agents     []string `json:"agents"`
	Global     bool     `json:"global"`
	AllowLocal bool     `json:"-"`
}
type Batch struct {
	Source  string   `json:"source"`
	Backend string   `json:"backend,omitempty"`
	Skills  []string `json:"skills"`
	Argv    []string `json:"argv"`
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

func ValidateRef(ref model.SkillRef, allowLocal bool) error {
	if !safeValue(ref.Name, 200) || ref.Name == "." || ref.Name == ".." || ref.Name == "*" {
		return fmt.Errorf("invalid skill name %q", ref.Name)
	}
	backend := BackendForSource(ref.Source)
	if backend == "" && strings.ContainsAny(ref.Name, `/\`) {
		return fmt.Errorf("invalid skill name %q", ref.Name)
	}
	if backend != "" {
		return nil
	}
	s := ref.Source
	if !safeValue(s, 2048) || strings.TrimSpace(s) != s {
		return fmt.Errorf("invalid skill source")
	}
	if repository.MatchString(s) && !strings.HasPrefix(s, ".") && !strings.Contains(s, "/.") {
		return nil
	}
	if strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err == nil && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path != "" {
			return nil
		}
	}
	if allowLocal && (filepath.IsAbs(s) || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")) {
		return nil
	}
	return fmt.Errorf("unsupported or untrusted skill source")
}

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
	names := map[string]string{}
	for _, ref := range refs {
		ref.Source = canonicalSource(ref.Source)
		if err := ValidateRef(ref, opts.AllowLocal); err != nil {
			return Plan{}, err
		}
		dest := canonicalSkillName(ref.Name)
		if dest == "" {
			return Plan{}, fmt.Errorf("invalid skill name %q", ref.Name)
		}
		if source, ok := names[dest]; ok && source != ref.Source {
			return Plan{}, fmt.Errorf("destination collision for skill %q between %q and %q", ref.Name, source, ref.Source)
		}
		names[dest] = ref.Source
		if seen[ref.Key()] {
			continue
		}
		seen[ref.Key()] = true
		if backend := BackendForSource(ref.Source); backend != "" {
			// Backend CLIs take a single skill argument per invocation.
			p.Batches = append(p.Batches, Batch{Source: ref.Source, Backend: backend, Skills: []string{ref.Name}})
		} else {
			n := len(p.Batches)
			if n == 0 || p.Batches[n-1].Source != ref.Source || p.Batches[n-1].Backend != "" {
				p.Batches = append(p.Batches, Batch{Source: ref.Source})
				n++
			}
			p.Batches[n-1].Skills = append(p.Batches[n-1].Skills, ref.Name)
		}
	}
	for i := range p.Batches {
		if p.Batches[i].Backend != "" {
			p.Batches[i].Argv = BackendArgs(p.Batches[i], agents, opts.Global)
			continue
		}
		p.Batches[i].Argv = AddArgs(p.Batches[i], agents, opts.Global)
	}
	return p, nil
}

// BackendArgs builds argv for backend-managed batches per each CLI's
// documented install contract (rev5 §2). Build guarantees one skill per batch.
func BackendArgs(b Batch, agents []string, global bool) []string {
	var args []string
	switch b.Backend {
	case BackendAskill:
		args = []string{"add", b.Skills[0]}
		for _, a := range agents {
			args = append(args, "-a", a)
		}
		if global {
			args = append(args, "-g")
		}
		return append(args, "-y")
	case BackendSkillfish:
		args = []string{"add", b.Skills[0]}
		for _, a := range agents {
			args = append(args, "--agent", a)
		}
		if global {
			args = append(args, "--global")
		} else {
			args = append(args, "--project")
		}
		return append(args, "-y")
	case BackendSmithery:
		args = []string{"skill", "add", b.Skills[0]}
		for _, a := range agents {
			args = append(args, "--agent", a)
		}
		if global {
			args = append(args, "-g")
		}
		return args
	default:
		return AddArgs(b, agents, global)
	}
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

type Runner interface {
	Run(context.Context, string, string, []string, io.Writer) error
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s execution failed: %w", name, err)
	}
	return nil
}

func Preflight(ctx context.Context) error {
	for _, name := range []string{"node", "npx"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("installation requires %s: %w", name, err)
		}
	}
	out, err := exec.CommandContext(ctx, "node", "--version").Output()
	if err != nil {
		return fmt.Errorf("check Node version: %w", err)
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), ".")
	if len(parts) < 3 {
		return fmt.Errorf("unrecognized Node version")
	}
	nums := make([]int, 3)
	for i := range nums {
		nums[i], err = strconv.Atoi(parts[i])
		if err != nil {
			return fmt.Errorf("unrecognized Node version")
		}
	}
	if nums[0] < 22 || (nums[0] == 22 && nums[1] < 20) {
		return fmt.Errorf("%s requires Node >=%s", Package, MinimumNode)
	}
	return nil
}

// PreflightBackends verifies every backend CLI referenced by the plan exists
// on PATH before any state mutation.
func PreflightBackends(p Plan) error {
	seen := map[string]bool{}
	for _, b := range p.Batches {
		if b.Backend == "" || seen[b.Backend] {
			continue
		}
		seen[b.Backend] = true
		binary, ok := backendBinaries[b.Backend]
		if !ok {
			return fmt.Errorf("unknown backend %q", b.Backend)
		}
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("backend %s requires %s on PATH: %w", b.Backend, binary, err)
		}
	}
	return nil
}

func Execute(ctx context.Context, p Plan, runner Runner, out io.Writer) error {
	if len(p.Batches) == 0 {
		return nil
	}
	if runner == nil {
		if err := Preflight(ctx); err != nil {
			return err
		}
		if err := PreflightBackends(p); err != nil {
			return err
		}
		runner = ExecRunner{}
	}
	if out == nil {
		out = io.Discard
	}
	state, err := ReadState(p.Root)
	if err != nil {
		return err
	}
	for _, b := range p.Batches {
		for _, name := range b.Skills {
			for _, e := range state.Entries {
				if canonicalSkillName(e.Name) == canonicalSkillName(name) && canonicalSource(e.Source) != canonicalSource(b.Source) && e.Scope == p.Scope {
					for _, a := range p.Agents {
						if slices.Contains(e.Agents, a) {
							return fmt.Errorf("tracked destination collision for %q", name)
						}
					}
				}
			}
		}
	}
	for _, b := range p.Batches {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Rebuild arguments from validated plan fields; never execute caller-provided argv.
		refs := make([]model.SkillRef, len(b.Skills))
		for i, n := range b.Skills {
			refs[i] = model.SkillRef{Source: b.Source, Name: n}
		}
		if _, err := Build(p.Root, refs, Options{Agents: p.Agents, Global: p.Scope == "global", AllowLocal: true}); err != nil {
			return err
		}
		if b.Backend != "" {
			if err := runner.Run(ctx, p.Root, backendBinaries[b.Backend], BackendArgs(b, p.Agents, p.Scope == "global"), out); err != nil {
				return err
			}
		} else if err := runner.Run(ctx, p.Root, "npx", AddArgs(b, p.Agents, p.Scope == "global"), out); err != nil {
			return err
		}
		if err := TrackBatch(p, &state, b); err != nil {
			return err
		}
		if err := WriteState(p.Root, state); err != nil {
			return err
		}
	}
	return nil
}
