package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/rigerc/sift/internal/model"
)

type cachedSearch struct {
	calls int
	err   error
}

func (b *cachedSearch) Name() string { return "cache-test" }
func (b *cachedSearch) Search(context.Context, Query) ([]ExternalSuggestion, error) {
	b.calls++
	if b.err != nil {
		return nil, b.err
	}
	return []ExternalSuggestion{{Skill: model.SkillRef{Source: "acme/repo", Name: "helper"}, SourceBackend: b.Name(), ExternalScore: 0.7}}, nil
}

func TestSearchOnlyRegistryRetainsFreshCacheAndStaleFallback(t *testing.T) {
	b := &cachedSearch{}
	r := &Registry{entries: []entry{{backend: b, enabled: CapSearch}}}
	cache := NewCache(t.TempDir())
	r.SetCache(cache, time.Hour)
	q := Query{Unresolved: []model.Observation{{Value: "novel"}}, RulesHash: "rules-v1", Limit: 2}
	first, errs := r.Search(context.Background(), q)
	if len(errs) != 0 || len(first) != 1 || b.calls != 1 {
		t.Fatalf("initial search: %+v %v calls=%d", first, errs, b.calls)
	}
	second, errs := r.Search(context.Background(), q)
	if len(errs) != 0 || !reflect.DeepEqual(first, second) || b.calls != 1 {
		t.Fatalf("fresh search cache changed: %+v %v calls=%d", second, errs, b.calls)
	}
	// Changing the rules hash must not reuse the previous search response.
	other := q
	other.RulesHash = "rules-v2"
	if _, errs := r.Search(context.Background(), other); len(errs) != 0 || b.calls != 2 {
		t.Fatalf("rules-hash isolation: %v calls=%d", errs, b.calls)
	}
	// Force expiry without waiting for wall-clock TTLs.
	request, _ := json.Marshal(q)
	sum := sha256.Sum256(request)
	key := CacheKey(b.Name(), "search", hex.EncodeToString(sum[:]), q.RulesHash)
	entry, ok, err := cache.Get(key)
	if err != nil || !ok {
		t.Fatalf("cached response missing: %+v %v", entry, err)
	}
	if err := cache.Put(key, entry.Payload, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	b.err = errors.New("provider down")
	stale, errs := r.Search(context.Background(), q)
	if len(errs) != 0 || len(stale) != 1 || !stale[0].Stale || b.calls != 3 {
		t.Fatalf("stale cache fallback changed: %+v %v calls=%d", stale, errs, b.calls)
	}
	missing := q
	missing.RulesHash = "uncached"
	if got, errs := r.Search(context.Background(), missing); len(got) != 0 || len(errs) != 1 {
		t.Fatalf("uncached failure must be advisory error: %+v %v", got, errs)
	}
}

func TestRegistryRejectsRemovedCapabilityBits(t *testing.T) {
	Register("search-only-cap-test", func(c Config) (Backend, error) { return fakeBackend{name: c.Name}, nil })
	for _, capability := range []Capability{2, 3, 255} {
		if _, err := NewRegistry([]Config{{Name: "search", Type: "search-only-cap-test", Capabilities: capability}}); err == nil {
			t.Errorf("accepted removed/unknown capability bits %d", capability)
		}
	}
}
