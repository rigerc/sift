package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cache is a deterministic, capability-aware response cache. Callers may use
// stale discovery entries after an online request fails; validation callers
// must reject stale entries themselves via Fresh.
type (
	Cache      struct{ dir string }
	CacheEntry struct {
		Key      string          `json:"key"`
		StoredAt time.Time       `json:"storedAt"`
		Payload  json.RawMessage `json:"payload"`
	}
)

func NewCache(dir string) *Cache { return &Cache{dir: dir} }
func CacheKey(backendName, capability, requestHash, rulesHash string) string {
	sum := sha256.Sum256([]byte(backendName + "\x00" + capability + "\x00" + requestHash + "\x00" + rulesHash))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) Get(key string) (CacheEntry, bool, error) {
	if c == nil || c.dir == "" {
		return CacheEntry{}, false, nil
	}
	data, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return CacheEntry{}, false, nil
	}
	if err != nil {
		return CacheEntry{}, false, err
	}
	var e CacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return CacheEntry{}, false, err
	}
	return e, true, nil
}

func (c *Cache) Put(key string, payload []byte, now time.Time) error {
	if c == nil || c.dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	e := CacheEntry{Key: key, StoredAt: now.UTC(), Payload: append([]byte(nil), payload...)}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp := filepath.Join(c.dir, "."+key+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(c.dir, key+".json"))
}

func Fresh(e CacheEntry, ttl time.Duration, now time.Time) bool {
	return ttl <= 0 || !e.StoredAt.IsZero() && now.Sub(e.StoredAt) <= ttl
}
