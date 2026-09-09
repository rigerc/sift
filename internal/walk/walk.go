// Package walk performs the single filesystem pass shared by all detectors.
package walk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

var defaultSkip = map[string]bool{"node_modules": true, ".git": true, "dist": true, "build": true, "target": true, "vendor": true, ".venv": true, "__pycache__": true, ".next": true, "coverage": true, ".idea": true, ".vscode": true, "bin": true, "obj": true, "Pods": true, ".terraform": true}

type Options struct {
	MaxDepth int
	SkipDirs []string
}
type File struct {
	Path string
	Ext  string
	Size int64
}
type Member struct {
	Root  string
	Files []File
}
type Result struct {
	Root      string
	Files     []File
	ExtCounts map[string]int
	NameIndex map[string][]string
	DirIndex  map[string][]string
	Members   []Member
	Warnings  []string
}

func Run(ctx context.Context, root string, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve root: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Result{}, fmt.Errorf("stat root: %w", err)
	}
	if !st.IsDir() {
		return Result{}, fmt.Errorf("root %q is not a directory", root)
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 8
	}
	skip := make(map[string]bool, len(defaultSkip)+len(opts.SkipDirs))
	for k := range defaultSkip {
		skip[k] = true
	}
	for _, k := range opts.SkipDirs {
		skip[k] = true
	}
	ignores, warnings := readIgnores(abs)
	r := Result{Root: abs, ExtCounts: map[string]int{}, NameIndex: map[string][]string{}, DirIndex: map[string][]string{}, Warnings: warnings}
	var candidates []string
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if walkErr != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", rel(abs, path), walkErr))
			return nil
		}
		relPath := rel(abs, path)
		if d.IsDir() {
			if b, e := os.ReadFile(filepath.Join(path, ".gitignore")); e == nil {
				ignores = append(ignores, parseIgnore(relPath, string(b))...)
			}
		}
		if relPath != "." && ignored(relPath, d.IsDir(), ignores) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if relPath != "." && skip[d.Name()] {
				return filepath.SkipDir
			}
			if depth(relPath) > opts.MaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if depth(relPath) > opts.MaxDepth {
			return nil
		}
		candidates = append(candidates, relPath)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("walk workspace: %w", err)
	}
	// WalkDir discovers paths once; metadata reads are bounded by a GOMAXPROCS
	// worker pool and results are sorted below before indexes are published.
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				if ctx.Err() != nil {
					continue
				}
				info, e := os.Stat(filepath.Join(abs, filepath.FromSlash(p)))
				if e != nil {
					mu.Lock()
					r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", p, e))
					mu.Unlock()
					continue
				}
				f := File{Path: p, Ext: strings.ToLower(filepath.Ext(filepath.Base(p))), Size: info.Size()}
				mu.Lock()
				r.Files = append(r.Files, f)
				r.ExtCounts[f.Ext]++
				r.NameIndex[filepath.Base(p)] = append(r.NameIndex[filepath.Base(p)], p)
				d := filepath.ToSlash(filepath.Dir(p))
				if d == "." {
					d = ""
				}
				r.DirIndex[d] = append(r.DirIndex[d], p)
				mu.Unlock()
			}
		}()
	}
	for _, p := range candidates {
		if ctx.Err() != nil {
			break
		}
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	sortResult(&r)
	r.Members = detectMembers(abs, r.Files, &r.Warnings)
	return r, nil
}

func sortResult(r *Result) {
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	for k := range r.NameIndex {
		sort.Strings(r.NameIndex[k])
	}
	for k := range r.DirIndex {
		sort.Strings(r.DirIndex[k])
	}
	sort.Strings(r.Warnings)
}

func rel(root, p string) string {
	x, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(x)
}

func depth(p string) int {
	if p == "." || p == "" {
		return 0
	}
	return strings.Count(filepath.ToSlash(p), "/") + 1
}

type ignoreRule struct {
	base    string
	pattern string
	dirOnly bool
	negate  bool
}

func readIgnores(root string) ([]ignoreRule, []string) {
	b, e := os.ReadFile(filepath.Join(root, ".gitignore"))
	if e != nil {
		if os.IsNotExist(e) {
			return nil, nil
		}
		return nil, []string{".gitignore: " + e.Error()}
	}
	var out []ignoreRule
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		if neg {
			line = strings.TrimPrefix(line, "!")
		}
		dir := strings.HasSuffix(line, "/")
		out = append(out, ignoreRule{".", strings.Trim(strings.TrimSuffix(line, "/"), "/"), dir, neg})
	}
	return out, nil
}

func parseIgnore(base, data string) []ignoreRule {
	var out []ignoreRule
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		if neg {
			line = strings.TrimPrefix(line, "!")
		}
		dir := strings.HasSuffix(line, "/")
		out = append(out, ignoreRule{base, strings.Trim(strings.TrimSuffix(line, "/"), "/"), dir, neg})
	}
	return out
}

func ignored(path string, isDir bool, rules []ignoreRule) bool {
	path = filepath.ToSlash(path)
	matched := false
	for _, r := range rules {
		if r.dirOnly && !isDir {
			continue
		}
		candidate := path
		if r.base != "." {
			candidate = strings.TrimPrefix(path, r.base+"/")
		}
		p := r.pattern
		if strings.ContainsAny(p, "*?[") {
			if ok, _ := filepath.Match(p, filepath.Base(candidate)); ok {
				matched = !r.negate
			}
			if ok, _ := filepath.Match(p, candidate); ok {
				matched = !r.negate
			}
		} else if candidate == p || strings.HasPrefix(candidate, p+"/") || filepath.Base(candidate) == p {
			matched = !r.negate
		}
	}
	return matched
}

func detectMembers(root string, files []File, warnings *[]string) []Member {
	set := map[string]bool{}
	by := map[string][]File{}
	for _, f := range files {
		switch filepath.Base(f.Path) {
		case "pnpm-workspace.yaml":
			for _, x := range parseYAMLGlobs(readFile(root, f.Path), warnings) {
				set[x] = true
			}
		case "lerna.json", "nx.json":
			set["."] = true
		case "go.work":
			for _, x := range parseGoWork(f.Path, readFile(root, f.Path), warnings) {
				set[x] = true
			}
		case "package.json":
			raw := readFile(root, f.Path)
			var probe map[string]any
			if json.Unmarshal([]byte(raw), &probe) != nil {
				*warnings = append(*warnings, f.Path+": malformed JSON")
			}
			for _, x := range parsePackageWorkspaces(raw) {
				set[x] = true
			}
		case "Cargo.toml":
			for _, x := range parseCargoWorkspace(readFile(root, f.Path), warnings) {
				set[x] = true
			}
		}
	}
	if len(set) == 0 {
		return []Member{{Root: ".", Files: append([]File(nil), files...)}}
	}
	roots := make([]string, 0, len(set))
	for x := range set {
		wild := strings.ContainsAny(x, "*?[")
		x = cleanMember(x)
		if x != "." {
			if !wild {
				roots = append(roots, x)
				continue
			}
			// Workspace globs such as packages/* are represented by their
			// parent during parsing; expand that parent against the cached
			// manifest so each member gets its own scope.
			expanded := false
			prefix := x + "/"
			if i := strings.IndexAny(prefix, "*?["); i >= 0 {
				prefix = strings.TrimSuffix(prefix[:i], "/") + "/"
			}
			seen := map[string]bool{}
			for _, f := range files {
				if strings.HasPrefix(f.Path, prefix) {
					rest := strings.TrimPrefix(f.Path, prefix)
					if i := strings.IndexByte(rest, '/'); i > 0 {
						child := strings.TrimSuffix(prefix, "/") + "/" + rest[:i]
						if !seen[child] {
							roots = append(roots, child)
							seen[child] = true
							expanded = true
						}
					}
				}
			}
			if !expanded {
				roots = append(roots, x)
			}
		}
	}
	sort.Strings(roots)
	if len(roots) > 1 {
		u := roots[:1]
		for _, x := range roots[1:] {
			if x != u[len(u)-1] {
				u = append(u, x)
			}
		}
		roots = u
	}
	for _, f := range files {
		m := "."
		for _, x := range roots {
			if f.Path == x || strings.HasPrefix(f.Path, x+"/") {
				m = x
				break
			}
		}
		by[m] = append(by[m], f)
	}
	out := []Member{{Root: ".", Files: by["."]}}
	for _, x := range roots {
		out = append(out, Member{Root: x, Files: by[x]})
	}
	return out
}

func cleanMember(x string) string {
	x = strings.TrimSpace(filepath.ToSlash(x))
	x = strings.TrimSuffix(x, "/")
	if x == "" {
		return "."
	}
	return filepath.Clean(x)
}

func readFile(root, path string) string {
	b, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if e != nil {
		return ""
	}
	return string(b)
}

func parsePackageWorkspaces(s string) []string {
	var v struct {
		Workspaces any `json:"workspaces"`
	}
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	switch x := v.Workspaces.(type) {
	case []any:
		var o []string
		for _, a := range x {
			if z, ok := a.(string); ok {
				o = append(o, globBase(z))
			}
		}
		return o
	case map[string]any:
		if a, ok := x["packages"].([]any); ok {
			var o []string
			for _, v := range a {
				if z, ok := v.(string); ok {
					o = append(o, globBase(z))
				}
			}
			return o
		}
	}
	return nil
}

func parseYAMLGlobs(s string, warnings *[]string) []string {
	var v struct {
		Packages []string `yaml:"packages"`
	}
	if yaml.Unmarshal([]byte(s), &v) != nil {
		*warnings = append(*warnings, "pnpm-workspace.yaml: malformed YAML")
		return nil
	}
	return v.Packages
}

func parseGoWork(name, s string, warnings *[]string) []string {
	w, e := modfile.ParseWork(name, []byte(s), nil)
	if e != nil {
		*warnings = append(*warnings, name+": "+e.Error())
		return nil
	}
	out := make([]string, 0, len(w.Use))
	for _, u := range w.Use {
		out = append(out, u.Path)
	}
	return out
}

func parseCargoWorkspace(s string, warnings *[]string) []string {
	var v struct {
		Workspace struct {
			Members []string `toml:"members"`
		} `toml:"workspace"`
	}
	if e := toml.Unmarshal([]byte(s), &v); e != nil {
		*warnings = append(*warnings, "Cargo.toml: "+e.Error())
		return nil
	}
	return v.Workspace.Members
}

func globBase(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "\"'"))
	if !strings.ContainsAny(s, "*?[") {
		return cleanMember(s)
	}
	return cleanMember(s)
}
