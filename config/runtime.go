package config

import (
	"errors"
	"fmt"
	"os"
)

// RuntimeOverrides contains explicitly supplied flags, including false booleans.
type RuntimeOverrides struct {
	Catalog  *string
	MaxDepth *int
	Online   *bool
	Global   *bool
}

type EffectiveConfig struct {
	Config *Config
	Path   string
}

// LoadEffective applies builtin defaults < file < supported environment < flags.
// A missing default file is normal; an explicit missing file is an error.
func LoadEffective(path string, explicitPath bool, overrides RuntimeOverrides) (*EffectiveConfig, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg, err := Load(path)
	if err != nil {
		if !errors.Is(err, ErrConfigNotFound) || explicitPath {
			return nil, fmt.Errorf("config: load %s: %w", path, err)
		}
		cfg = DefaultConfig()
	}
	if catalog, ok := os.LookupEnv("SIFT_CATALOG_URL"); ok {
		cfg.Scan.Catalog = catalog
	}
	if overrides.Catalog != nil {
		cfg.Scan.Catalog = *overrides.Catalog
	}
	if overrides.MaxDepth != nil {
		cfg.Scan.MaxDepth = *overrides.MaxDepth
	}
	if overrides.Online != nil {
		cfg.Scan.Online = *overrides.Online
	}
	if overrides.Global != nil {
		cfg.Install.Global = *overrides.Global
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &EffectiveConfig{Config: cfg, Path: path}, nil
}
