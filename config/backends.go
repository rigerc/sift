package config

import (
	"errors"
	"fmt"
	"go-s/internal/backend"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type BackendFile struct {
	Strategy string         `yaml:"strategy"`
	Backends []BackendEntry `yaml:"backends"`
}
type BackendEntry struct {
	Name, Type, URL, Auth string
	Capabilities          []string `yaml:"capabilities"`
	Timeout               Duration `yaml:"timeout"`
	CacheTTL              Duration `yaml:"cache_ttl"`
}
type Duration struct{ time.Duration }

// DefaultBackends is intentionally empty: remote discovery is opt-in.
func DefaultBackends() BackendFile { return BackendFile{Strategy: "fanout"} }

// MergeBackends overlays entries by name while preserving base priority for
// entries that are not overridden and override priority for replacements.
func MergeBackends(base, override BackendFile) BackendFile {
	out := base
	if override.Strategy != "" {
		out.Strategy = override.Strategy
	}
	positions := map[string]int{}
	for i, entry := range out.Backends {
		positions[entry.Name] = i
	}
	for _, entry := range override.Backends {
		if i, ok := positions[entry.Name]; ok {
			out.Backends[i] = entry
		} else {
			positions[entry.Name] = len(out.Backends)
			out.Backends = append(out.Backends, entry)
		}
	}
	return out
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = parsed
	return nil
}

func LoadBackends(explicit, configDir string) (BackendFile, string, error) {
	path := explicit
	if path == "" {
		path = os.Getenv("SKILLSCAN_BACKENDS")
	}
	if path == "" && configDir != "" {
		path = filepath.Join(configDir, "backends.yaml")
	}
	if path == "" {
		return BackendFile{Strategy: "fanout"}, "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && explicit == "" {
			return BackendFile{Strategy: "fanout"}, "", nil
		}
		return BackendFile{}, path, fmt.Errorf("read backends config: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil {
		return BackendFile{}, path, fmt.Errorf("read backends config: %w", err)
	}
	if len(data) > 1<<20 {
		return BackendFile{}, path, fmt.Errorf("backends config exceeds 1 MiB")
	}
	var cfg BackendFile
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return BackendFile{}, path, fmt.Errorf("parse backends config: %w", err)
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "fanout"
	}
	if cfg.Strategy != "fanout" && cfg.Strategy != "first-hit" {
		return BackendFile{}, path, fmt.Errorf("invalid backend strategy %q", cfg.Strategy)
	}
	seen := map[string]bool{}
	for i := range cfg.Backends {
		b := &cfg.Backends[i]
		if b.Name == "" || b.Type == "" || seen[b.Name] {
			return BackendFile{}, path, fmt.Errorf("invalid duplicate/empty backend name %q", b.Name)
		}
		seen[b.Name] = true
		if b.Auth != "" && !strings.HasPrefix(b.Auth, "env:") {
			return BackendFile{}, path, fmt.Errorf("backend %s auth must be env:NAME", b.Name)
		}
		if b.Timeout.Duration < 0 || b.CacheTTL.Duration < 0 {
			return BackendFile{}, path, fmt.Errorf("backend %s duration cannot be negative", b.Name)
		}
	}
	return cfg, path, nil
}

func ResolveBackendAuth(reference string) (string, error) {
	if reference == "" {
		return "", nil
	}
	if !strings.HasPrefix(reference, "env:") {
		return "", errors.New("auth must be an env: reference")
	}
	name := strings.TrimPrefix(reference, "env:")
	if name == "" || strings.ContainsAny(name, "\x00=\r\n") {
		return "", errors.New("invalid auth environment reference")
	}
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("auth environment variable %s is not set", name)
	}
	return value, nil
}

func (f BackendFile) Runtime() ([]backend.Config, backend.Strategy, error) {
	out := make([]backend.Config, 0, len(f.Backends))
	for _, b := range f.Backends {
		caps := backend.Capability(0)
		for _, name := range b.Capabilities {
			switch name {
			case "search":
				caps |= backend.CapSearch
			case "validate":
				caps |= backend.CapValidate
			case "report":
				caps |= backend.CapReport
			case "catalog":
				caps |= backend.CapCatalog
			default:
				return nil, "", fmt.Errorf("backend %s has unknown capability %q", b.Name, name)
			}
		}
		out = append(out, backend.Config{Name: b.Name, Type: b.Type, URL: b.URL, Auth: b.Auth, Capabilities: caps, Timeout: b.Timeout.Duration, CacheTTL: b.CacheTTL.Duration})
	}
	return out, backend.Strategy(f.Strategy), nil
}
