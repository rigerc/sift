package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// RuntimeOverrides are one-invocation values and are never persisted.
type RuntimeOverrides struct {
	Debug    *bool
	LogLevel *string
}

// EffectiveConfig keeps file values separate from effective runtime values.
type EffectiveConfig struct {
	Config       *Config
	Persisted    *Config
	Path         string
	Warnings     []string
	ExplicitPath bool
}

// LoadEffective applies defaults, file, supported environment variables, and
// explicitly changed flags in that order. Missing default files are valid.
func LoadEffective(path string, explicitPath bool, overrides RuntimeOverrides) (*EffectiveConfig, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	persisted := DefaultConfig()
	warnings := []string(nil)
	if loaded, err := Load(path); err == nil {
		persisted = loaded
		if raw, readErr := os.ReadFile(path); readErr == nil {
			warnings = unknownKeys(raw)
		}
	} else if !errors.Is(err, ErrConfigNotFound) || explicitPath {
		return nil, fmt.Errorf("config: load %s: %w", path, err)
	}
	effective := *persisted
	if raw, ok := os.LookupEnv("SKILLSCAN_DEBUG"); ok {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("config: SKILLSCAN_DEBUG must be true or false: %w", err)
		}
		effective.Debug = value
	}
	if raw, ok := os.LookupEnv("SKILLSCAN_LOG_LEVEL"); ok {
		effective.LogLevel = strings.ToLower(strings.TrimSpace(raw))
	}
	if overrides.Debug != nil {
		effective.Debug = *overrides.Debug
	}
	if overrides.LogLevel != nil {
		effective.LogLevel = strings.ToLower(strings.TrimSpace(*overrides.LogLevel))
	}
	if err := effective.Validate(); err != nil {
		return nil, err
	}
	return &EffectiveConfig{Config: &effective, Persisted: persisted, Path: path, Warnings: warnings, ExplicitPath: explicitPath}, nil
}

func unknownKeys(data []byte) []string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	known := map[string]bool{"configVersion": true, "logLevel": true, "debug": true, "ui": true, "editor": true, "network": true, "notifications": true, "scan": true, "install": true, "app": true}
	result := make([]string, 0)
	for key := range raw {
		if !known[key] {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}
