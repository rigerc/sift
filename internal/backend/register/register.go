// Package register installs the built-in backend adapter constructors.
package register

import (
	"fmt"
	"go-s/internal/backend"
	"go-s/internal/backend/catalog"
	"go-s/internal/backend/githubtrees"
	"go-s/internal/backend/semantic"
	"go-s/internal/backend/skillssh"
)

func init() {
	backend.Register("skills.sh", skillssh.New)
	backend.Register("github-trees", githubtrees.New)
	backend.Register("semantic", semantic.New)
	backend.Register("catalog", catalog.New)
}

// New constructs a registry with all built-in adapter types registered.
func New(configs []backend.Config, strategy backend.Strategy) (*backend.Registry, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	return backend.NewRegistry(configs, strategy)
}
