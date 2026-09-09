package install

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Entry struct {
	Source        string   `json:"source"`
	Name          string   `json:"name"`
	Agents        []string `json:"agents"`
	Scope         string   `json:"scope"`
	ContentSHA256 string   `json:"contentSha256"`
	InstalledAt   string   `json:"installedAt"`
	Revision      string   `json:"revision,omitempty"`
}
type State struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}
type Status struct {
	Entry
	Status string `json:"status"`
}

func ReadState(root string) (State, error) {
	s := State{Version: 1, Entries: []Entry{}}
	f, err := os.Open(filepath.Join(root, ".skillscan", "lock.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(io.LimitReader(f, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("read skillscan state: %w", err)
	}
	if s.Version != 1 {
		return s, fmt.Errorf("unsupported skillscan state version %d", s.Version)
	}
	return s, nil
}

func WriteState(root string, s State) error {
	slices.SortFunc(s.Entries, func(a, b Entry) int {
		return strings.Compare(a.Scope+"\x00"+a.Source+"\x00"+a.Name, b.Scope+"\x00"+b.Source+"\x00"+b.Name)
	})
	dir := filepath.Join(root, ".skillscan")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(dir, ".lock-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "lock.json"))
}

func skillPath(root, name, scope string) (string, error) {
	name = canonicalSkillName(name)
	if name == "" {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	if scope == "global" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".agents", "skills", name), nil
	}
	return filepath.Join(root, ".agents", "skills", name), nil
}

func HashDirectory(dir string) (string, error) {
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	files := []string{}
	err = filepath.WalkDir(canonical, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains symlink %q", path)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("skill contains nonregular file")
		}
		rel, e := filepath.Rel(canonical, path)
		if e != nil {
			return e
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.Sort(files)
	h := sha256.New()
	var length [8]byte
	for _, rel := range files {
		binary.BigEndian.PutUint64(length[:], uint64(len(rel)))
		h.Write(length[:])
		_, _ = io.WriteString(h, rel)
		f, err := os.Open(filepath.Join(canonical, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return "", err
		}
		binary.BigEndian.PutUint64(length[:], uint64(info.Size()))
		_, _ = h.Write(length[:])
		_, err = io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func TrackBatch(p Plan, s *State, b Batch) error {
	for _, name := range b.Skills {
		path, err := skillPath(p.Root, name, p.Scope)
		if err != nil {
			return err
		}
		hash, err := HashDirectory(path)
		if err != nil {
			return fmt.Errorf("installed skill %q cannot be hashed: %w", name, err)
		}
		entry := Entry{Source: b.Source, Name: name, Agents: slices.Clone(p.Agents), Scope: p.Scope, ContentSHA256: hash, InstalledAt: time.Now().UTC().Format(time.RFC3339)}
		found := false
		for i, e := range s.Entries {
			if canonicalSource(e.Source) == canonicalSource(entry.Source) && canonicalSkillName(e.Name) == canonicalSkillName(name) && e.Scope == entry.Scope {
				entry.Agents = append(entry.Agents, e.Agents...)
				slices.Sort(entry.Agents)
				entry.Agents = slices.Compact(entry.Agents)
				s.Entries[i] = entry
				found = true
				break
			}
		}
		if !found {
			s.Entries = append(s.Entries, entry)
		}
	}
	return nil
}

func Inspect(root string) ([]Status, error) {
	s, err := ReadState(root)
	if err != nil {
		return nil, err
	}
	out := []Status{}
	for _, e := range s.Entries {
		path, err := skillPath(root, e.Name, e.Scope)
		if err != nil {
			return nil, err
		}
		hash, err := HashDirectory(path)
		st := "installed"
		if errors.Is(err, os.ErrNotExist) {
			st = "missing"
		} else if err != nil {
			st = "unreadable"
		} else if hash != e.ContentSHA256 {
			st = "modified"
		}
		out = append(out, Status{Entry: e, Status: st})
	}
	return out, nil
}

type UpdateOptions struct {
	Names   []string
	Global  bool
	Project bool
}

func Update(ctx context.Context, root string, runner Runner, out io.Writer) error {
	return UpdateSelected(ctx, root, UpdateOptions{}, runner, out)
}

func UpdateSelected(ctx context.Context, root string, opts UpdateOptions, runner Runner, out io.Writer) error {
	if opts.Global && opts.Project {
		return fmt.Errorf("choose global or project scope")
	}
	for _, name := range opts.Names {
		if !safeValue(name, 200) || strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("invalid update skill name")
		}
	}
	if runner == nil {
		if err := Preflight(ctx); err != nil {
			return err
		}
		runner = ExecRunner{}
	}
	if out == nil {
		out = io.Discard
	}
	args := append([]string{"--yes", Package, "update"}, opts.Names...)
	args = append(args, "--yes")
	if opts.Global {
		args = append(args, "--global")
	}
	if opts.Project {
		args = append(args, "--project")
	}
	if err := runner.Run(ctx, root, "npx", args, out); err != nil {
		return err
	}
	s, err := ReadState(root)
	if err != nil {
		return err
	}
	for i, e := range s.Entries {
		path, err := skillPath(root, e.Name, e.Scope)
		if err != nil {
			return err
		}
		if hash, err := HashDirectory(path); err == nil {
			s.Entries[i].ContentSHA256 = hash
		}
	}
	return WriteState(root, s)
}
