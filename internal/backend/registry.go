package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go-s/internal/model"
	"go-s/internal/rules"
	"sort"
	"sync"
	"time"
)

// Registry preserves configured priority order and only exposes capabilities
// explicitly enabled in configuration.
type Registry struct {
	entries  []entry
	strategy Strategy
	cache    *Cache
	cacheTTL time.Duration
}
type entry struct {
	backend Backend
	enabled Capability
	timeout time.Duration
}
type Factory func(Config) (Backend, error)

var factories = struct {
	sync.RWMutex
	m map[string]Factory
}{m: make(map[string]Factory)}

func Register(kind string, f Factory) {
	factories.Lock()
	defer factories.Unlock()
	factories.m[kind] = f
}

// FactoryFor returns the registered constructor for a backend type. Setup
// tooling uses it to build adapters for types that are not yet configured.
func FactoryFor(kind string) (Factory, bool) {
	factories.RLock()
	defer factories.RUnlock()
	f, ok := factories.m[kind]
	return f, ok
}

func NewRegistry(configs []Config, strategy Strategy) (*Registry, error) {
	if strategy == "" {
		strategy = StrategyFanout
	}
	if strategy != StrategyFanout && strategy != StrategyFirstHit {
		return nil, fmt.Errorf("invalid backend strategy %q", strategy)
	}
	r := &Registry{strategy: strategy}
	seen := map[string]bool{}
	for _, cfg := range configs {
		if cfg.Name == "" || seen[cfg.Name] {
			return nil, fmt.Errorf("duplicate or empty backend name %q", cfg.Name)
		}
		seen[cfg.Name] = true
		factories.RLock()
		f := factories.m[cfg.Type]
		factories.RUnlock()
		if f == nil {
			return nil, fmt.Errorf("unknown backend type %q", cfg.Type)
		}
		b, err := f(cfg)
		if err != nil {
			return nil, fmt.Errorf("backend %s: %w", cfg.Name, err)
		}
		if !capabilitySupported(b, cfg.Capabilities) {
			return nil, fmt.Errorf("backend %s does not implement configured capability", cfg.Name)
		}
		r.entries = append(r.entries, entry{backend: b, enabled: cfg.Capabilities, timeout: cfg.Timeout})
	}
	return r, nil
}
func (r *Registry) SetCache(cache *Cache, ttl time.Duration) { r.cache, r.cacheTTL = cache, ttl }
func capabilitySupported(b Backend, caps Capability) bool {
	if caps.Has(CapSearch) {
		if _, ok := b.(Searcher); !ok {
			return false
		}
	}
	if caps.Has(CapValidate) {
		if _, ok := b.(Validator); !ok {
			return false
		}
	}
	if caps.Has(CapReport) {
		if _, ok := b.(Reporter); !ok {
			return false
		}
	}
	if caps.Has(CapCatalog) {
		if _, ok := b.(CatalogProvider); !ok {
			return false
		}
	}
	return true
}

func (r *Registry) Get(name string) (Backend, error) {
	for _, e := range r.entries {
		if e.backend.Name() == name {
			return e.backend, nil
		}
	}
	return nil, fmt.Errorf("backend %q not found", name)
}

func (r *Registry) ByCap(c Capability) []Backend {
	out := []Backend{}
	for _, e := range r.entries {
		if !e.enabled.Has(c) {
			continue
		}
		if c == CapSearch {
			if _, ok := e.backend.(Searcher); !ok {
				continue
			}
		}
		if c == CapValidate {
			if _, ok := e.backend.(Validator); !ok {
				continue
			}
		}
		if c == CapReport {
			if _, ok := e.backend.(Reporter); !ok {
				continue
			}
		}
		if c == CapCatalog {
			if _, ok := e.backend.(CatalogProvider); !ok {
				continue
			}
		}
		out = append(out, e.backend)
	}
	return out
}

func (r *Registry) Search(ctx context.Context, q Query) ([]ExternalSuggestion, []error) {
	backends := r.ByCap(CapSearch)
	type answer struct {
		i   int
		v   []ExternalSuggestion
		err error
	}
	ch := make(chan answer, len(backends))
	var wg sync.WaitGroup
	if r.strategy == StrategyFirstHit {
		var errs []error
		for _, b := range backends {
			callCtx, cancel := r.entryContext(ctx, b)
			v, err := r.searchOne(callCtx, b, q)
			cancel()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(v) > 0 {
				return mergeSuggestions(v), errs
			}
		}
		return nil, errs
	}
	for i, b := range backends {
		wg.Add(1)
		go func(i int, b Backend) {
			defer wg.Done()
			callCtx, cancel := r.entryContext(ctx, b)
			v, err := r.searchOne(callCtx, b, q)
			cancel()
			ch <- answer{i, v, err}
		}(i, b)
	}
	wg.Wait()
	close(ch)
	all := make([][]ExternalSuggestion, len(backends))
	errsByIndex := make([]error, len(backends))
	for a := range ch {
		all[a.i] = a.v
		if a.err != nil {
			errsByIndex[a.i] = a.err
		}
	}
	errs := []error{}
	for _, err := range errsByIndex {
		if err != nil {
			errs = append(errs, err)
		}
	}
	merged := []ExternalSuggestion{}
	for _, v := range all {
		merged = append(merged, v...)
	}
	return mergeSuggestions(merged), errs
}

func (r *Registry) searchOne(ctx context.Context, b Backend, q Query) ([]ExternalSuggestion, error) {
	request, _ := json.Marshal(q)
	sum := sha256.Sum256(request)
	key := CacheKey(b.Name(), "search", hex.EncodeToString(sum[:]), q.RulesHash)
	var stale []ExternalSuggestion
	if r.cache != nil {
		if e, ok, err := r.cache.Get(key); err == nil && ok {
			_ = json.Unmarshal(e.Payload, &stale)
			if Fresh(e, r.cacheTTL, time.Now()) {
				return stale, nil
			}
		}
	}
	v, err := b.(Searcher).Search(ctx, q)
	if err == nil {
		if r.cache != nil {
			if data, marshalErr := json.Marshal(v); marshalErr == nil {
				_ = r.cache.Put(key, data, time.Now())
			}
		}
		return v, nil
	}
	if len(stale) > 0 {
		for i := range stale {
			stale[i].Stale = true
		}
		return stale, nil
	}
	return nil, err
}

func mergeSuggestions(in []ExternalSuggestion) []ExternalSuggestion {
	seen := map[string]ExternalSuggestion{}
	for _, v := range in {
		if v.Skill.Source == "" || v.Skill.Name == "" {
			continue
		}
		k := v.Skill.Key()
		if _, ok := seen[k]; !ok {
			seen[k] = v
		}
	}
	out := make([]ExternalSuggestion, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExternalScore != out[j].ExternalScore {
			return out[i].ExternalScore > out[j].ExternalScore
		}
		return out[i].Skill.Key() < out[j].Skill.Key()
	})
	return out
}

func (r *Registry) entryContext(ctx context.Context, backend Backend) (context.Context, context.CancelFunc) {
	for _, e := range r.entries {
		if e.backend.Name() == backend.Name() && e.timeout > 0 {
			return context.WithTimeout(ctx, e.timeout)
		}
	}
	return context.WithCancel(ctx)
}

func (r *Registry) Validate(ctx context.Context, skills []model.SkillRef) map[string]Validation {
	result := map[string]Validation{}
	for _, skill := range skills {
		result[skill.Key()] = Validation{Status: StatusUnknown}
	}
	for _, b := range r.ByCap(CapValidate) {
		callCtx, cancel := r.entryContext(ctx, b)
		v, err := b.(Validator).Validate(callCtx, skills)
		cancel()
		if err != nil {
			continue
		}
		for _, skill := range skills {
			key := skill.Key()
			answer, ok := v[key]
			if !ok || answer.Status == StatusUnknown {
				continue
			}
			if result[key].Status == StatusUnknown {
				result[key] = answer
			}
		}
	}
	return result
}

func (r *Registry) Catalog(ctx context.Context) (rules.Catalog, error) {
	for _, b := range r.ByCap(CapCatalog) {
		c, err := b.(CatalogProvider).Catalog(ctx)
		if err == nil {
			return c, nil
		}
	}
	return rules.Catalog{}, fmt.Errorf("no catalog provider succeeded")
}
