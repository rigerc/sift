// Package register installs the built-in backend adapter constructors.
package register

import (
	"fmt"
	"go-s/internal/backend"
	"go-s/internal/backend/askill"
	"go-s/internal/backend/catalog"
	"go-s/internal/backend/githubtrees"
	"go-s/internal/backend/semantic"
	"go-s/internal/backend/skillfish"
	"go-s/internal/backend/skillssh"
	"go-s/internal/backend/smithery"
)

func init() {
	backend.Register("skills.sh", skillssh.New)
	backend.Register("github-trees", githubtrees.New)
	backend.Register("semantic", semantic.New)
	backend.Register("catalog", catalog.New)
	backend.Register("askill", askill.New)
	backend.Register("skillfish", skillfish.New)
	backend.Register("smithery", smithery.New)
}

// New constructs a registry with all built-in adapter types registered.
func New(configs []backend.Config, strategy backend.Strategy) (*backend.Registry, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	return backend.NewRegistry(configs, strategy)
}

// CLIInfo describes a built-in local CLI backend for setup tooling.
type CLIInfo struct {
	Type, Binary, PinnedVersion string
}

// CLIBackends lists the built-in process-CLI backends. These run local
// binaries, so availability can be verified without configuration.
func CLIBackends() []CLIInfo {
	return []CLIInfo{
		{Type: "askill", Binary: askill.Binary, PinnedVersion: askill.PinnedVersion},
		{Type: "skillfish", Binary: skillfish.Binary, PinnedVersion: skillfish.PinnedVersion},
		{Type: "smithery", Binary: smithery.Binary, PinnedVersion: smithery.PinnedVersion},
	}
}
