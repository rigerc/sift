// Package register installs the built-in backend adapter constructors.
package register

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rigerc/sift/config"
	"github.com/rigerc/sift/internal/backend"
	"github.com/rigerc/sift/internal/backend/decimalai"
	"github.com/rigerc/sift/internal/backend/officialskills"
	"github.com/rigerc/sift/internal/backend/skillsmp"
	"github.com/rigerc/sift/internal/backend/skyll"
)

// searchCacheTTL bounds how long a discovery response is reused.
const searchCacheTTL = 24 * time.Hour

func init() {
	backend.Register("official-skills", officialskills.New)
	backend.Register("skyll", skyll.New)
	backend.Register("skillsmp", skillsmp.New)
	backend.Register("decimalai", decimalai.New)
}

// New constructs the registry with every built-in backend and wires the
// shared discovery search cache. There is no user configuration.
func New() (*backend.Registry, error) {
	registry, err := backend.NewRegistry(Builtins())
	if err != nil {
		return nil, err
	}
	if root := config.DefaultCacheDir(); root != "" {
		registry.SetCache(backend.NewCache(filepath.Join(root, "search")), searchCacheTTL)
	}
	return registry, nil
}

// Builtins returns the configuration of the shipped discovery backends. The
// HTTP adapters are seeded with their optional API tokens from the
// environment; an unset variable means an anonymous request.
func Builtins() []backend.Config {
	return []backend.Config{
		{
			Name:         "official-skills",
			Type:         "official-skills",
			Capabilities: backend.CapSearch,
			CacheDir:     config.DefaultCacheDir(),
		},
		{
			Name:         "skyll",
			Type:         "skyll",
			Capabilities: backend.CapSearch,
		},
		{
			Name:         "skillsmp",
			Type:         "skillsmp",
			Capabilities: backend.CapSearch,
			Auth:         os.Getenv("SKILLSMP_API_KEY"),
		},
		{
			Name:         "decimalai",
			Type:         "decimalai",
			Capabilities: backend.CapSearch,
			Auth:         os.Getenv("DECIMAL_API_KEY"),
		},
	}
}
