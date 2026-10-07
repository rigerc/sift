// Package config loads the read-only scanner configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrConfigNotFound = errors.New("configuration file not found")
	ErrInvalidConfig  = errors.New("invalid configuration")
)

// Config deliberately retains only scanner settings and the legacy scope key.
// Unknown legacy keys are ignored; loading never rewrites a file.
type Config struct {
	Scan    ScanConfig    `json:"scan"`
	Install InstallConfig `json:"install"`
}

type ScanConfig struct {
	Catalog  string `json:"catalog"`
	MaxDepth int    `json:"maxDepth"`
	Online   bool   `json:"online"`
}

type InstallConfig struct {
	Global bool `json:"global"`
}

func DefaultConfig() *Config {
	return &Config{Scan: ScanConfig{MaxDepth: 8, Online: true}}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading configuration: %w", err)
	}
	return LoadFromBytes(data)
}

func LoadFromBytes(data []byte) (*Config, error) {
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return nil, fmt.Errorf("%w: expected a JSON object", ErrInvalidConfig)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	// Old configurations used zero to mean the default scan depth.
	if cfg.Scan.MaxDepth == 0 {
		cfg.Scan.MaxDepth = 8
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Scan.MaxDepth < 1 || c.Scan.MaxDepth > 64 {
		return fmt.Errorf("%w: scan max depth must be between 1 and 64", ErrInvalidConfig)
	}
	return nil
}
