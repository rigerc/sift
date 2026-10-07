package officialskills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultBase is the raw GitHub URL hosting the registry files.
	DefaultBase = "https://raw.githubusercontent.com/rigerc/officialskills/main/registry"
	// DefaultTTL matches the 24h URL-resolution window of the registry generator.
	DefaultTTL = 24 * time.Hour
	// maxRegistryBytes bounds a single registry download.
	maxRegistryBytes = 64 << 20
)

// registryFiles are the documents merged, in priority order.
var registryFiles = []string{"official.json", "community.json"}

// Snapshot is the loaded registry state.
type Snapshot struct {
	Skills []Skill
	// Offline is true when at least one file was served from a stale cache
	// after a fetch failure.
	Offline bool
}

// Loader fetches registry files with a local disk cache.
type Loader struct {
	Base       string
	CacheDir   string
	HTTPClient *http.Client
	Timeout    time.Duration
	TTL        time.Duration
}

// NewLoader builds a Loader with defaults. cacheDir "" disables caching.
func NewLoader(base, cacheDir string) *Loader {
	if base == "" {
		base = DefaultBase
	}
	return &Loader{
		Base:       base,
		CacheDir:   cacheDir,
		HTTPClient: &http.Client{},
		Timeout:    20 * time.Second,
		TTL:        DefaultTTL,
	}
}

// Load returns the merged, installable-only skill list. A fresh cache wins;
// otherwise the files are refetched. On fetch failure a stale cached copy is
// served and Offline is set.
func (l *Loader) Load(ctx context.Context) (Snapshot, error) {
	var outputs []Output
	stale := false
	for _, name := range registryFiles {
		out, isStale, err := l.loadOne(ctx, name)
		if err != nil {
			return Snapshot{}, err
		}
		stale = stale || isStale
		outputs = append(outputs, out)
	}
	return Snapshot{Skills: InstallableOnly(Flatten(outputs...)), Offline: stale}, nil
}

func (l *Loader) loadOne(ctx context.Context, name string) (Output, bool, error) {
	if data, ok := l.readCache(name, false); ok {
		if out, err := ParseOutput(data); err == nil {
			return out, false, nil
		}
	}
	data, err := l.fetch(ctx, name)
	if err == nil {
		if out, perr := ParseOutput(data); perr == nil {
			_ = l.writeCache(name, data)
			return out, false, nil
		}
		if cached, ok := l.readCache(name, true); ok {
			if out, perr := ParseOutput(cached); perr == nil {
				return out, true, nil
			}
		}
		return Output{}, false, fmt.Errorf("fetch %s: unparseable response", name)
	}
	if cached, ok := l.readCache(name, true); ok {
		if out, perr := ParseOutput(cached); perr == nil {
			return out, true, nil
		}
	}
	return Output{}, false, fmt.Errorf("fetch %s: %w (no cached copy available)", name, err)
}

func (l *Loader) url(name string) string { return l.Base + "/" + name }

func (l *Loader) fetch(ctx context.Context, name string) ([]byte, error) {
	client := l.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	timeout := l.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, l.url(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes))
}

func (l *Loader) cachePath(name string) string {
	return filepath.Join(l.CacheDir, name)
}

func (l *Loader) readCache(name string, allowStale bool) ([]byte, bool) {
	if l.CacheDir == "" {
		return nil, false
	}
	path := l.cachePath(name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	ttl := l.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if !allowStale && time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

func (l *Loader) writeCache(name string, data []byte) error {
	if l.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(l.CacheDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(l.cachePath(name), data, 0o644)
}
