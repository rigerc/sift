package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go-s/internal/model"
	"sort"
	"sync"
	"time"
)

// Registry preserves built-in priority order and only exposes capabilities
// explicitly enabled per backend.
type Registry struct {
	entries  []entry
	cache    *Cache
	cacheTTL time.Duration
}
type entry struct {
	backend Backend
	enabled Capability
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

func NewRegistry(configs []Config) (*Registry, error) {
	r := &Registry{}
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
		r.entries = append(r.entries, entry{backend: b, enabled: cfg.Capabilities})
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
	for i, b := range backends {
		wg.Add(1)
		go func(i int, b Backend) {
			defer wg.Done()
			callCtx, cancel := r.entryContext(ctx)
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
	return mergeSuggestions(all, q.Limit), errs
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

func mergeSuggestions(all [][]ExternalSuggestion, limit int) []ExternalSuggestion {
	type ranked struct {
		value        ExternalSuggestion
		backendIndex int
		resultIndex  int
		score        float64
	}

	seen := map[string]bool{}
	rankedOut := make([]ranked, 0)
	for backendIndex, values := range all {
		for resultIndex, v := range values {
			if v.Skill.Source == "" || v.Skill.Name == "" {
				continue
			}
			k := v.Skill.Key()
			if seen[k] {
				continue
			}
			seen[k] = true

			upstream := clampExternalScore(v.ExternalScore)
			position := 1.0 / (1.0 + 0.25*float64(resultIndex))
			combined := 0.75*position + 0.25*upstream
			if v.Stale {
				combined -= 0.05
			}
			rankedOut = append(rankedOut, ranked{
				value: v, backendIndex: backendIndex, resultIndex: resultIndex, score: combined,
			})
		}
	}

	sort.SliceStable(rankedOut, func(i, j int) bool {
		if rankedOut[i].score != rankedOut[j].score {
			return rankedOut[i].score > rankedOut[j].score
		}
		if rankedOut[i].value.ExternalScore != rankedOut[j].value.ExternalScore {
			return rankedOut[i].value.ExternalScore > rankedOut[j].value.ExternalScore
		}
		if rankedOut[i].backendIndex != rankedOut[j].backendIndex {
			return rankedOut[i].backendIndex < rankedOut[j].backendIndex
		}
		if rankedOut[i].resultIndex != rankedOut[j].resultIndex {
			return rankedOut[i].resultIndex < rankedOut[j].resultIndex
		}
		return rankedOut[i].value.Skill.Key() < rankedOut[j].value.Skill.Key()
	})

	if limit > 0 && len(rankedOut) > limit {
		rankedOut = rankedOut[:limit]
	}
	out := make([]ExternalSuggestion, len(rankedOut))
	for i := range rankedOut {
		out[i] = rankedOut[i].value
	}
	return out
}

func clampExternalScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func (r *Registry) entryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}

func (r *Registry) Validate(ctx context.Context, skills []model.SkillRef) map[string]Validation {
	result := map[string]Validation{}
	for _, skill := range skills {
		result[skill.Key()] = Validation{Status: StatusUnknown}
	}
	for _, b := range r.ByCap(CapValidate) {
		callCtx, cancel := r.entryContext(ctx)
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
