package register

import (
	"testing"

	"github.com/rigerc/sift/internal/backend"
)

func TestBuiltinsResolveAndConstruct(t *testing.T) {
	builtins := Builtins()
	if len(builtins) != 4 {
		t.Fatalf("builtins = %d, want 4", len(builtins))
	}
	for _, cfg := range builtins {
		t.Run(cfg.Type, func(t *testing.T) {
			factory, ok := backend.FactoryFor(cfg.Type)
			if !ok {
				t.Fatalf("backend type %q not registered", cfg.Type)
			}
			adapter, err := factory(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := adapter.(backend.Searcher); !ok {
				t.Fatalf("backend %s does not implement Searcher", cfg.Name)
			}
			if _, ok := adapter.(backend.VersionProber); !ok {
				t.Fatalf("backend %s does not implement VersionProber", cfg.Name)
			}
		})
	}
}

func TestNewRegistryHasFourSearchBackends(t *testing.T) {
	registry, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(registry.ByCap(backend.CapSearch)); got != 4 {
		t.Fatalf("search backends = %d, want 4", got)
	}
}

func TestBuiltinsAuthFromEnvironment(t *testing.T) {
	t.Setenv("SKILLSMP_API_KEY", "")
	t.Setenv("DECIMAL_API_KEY", "")
	for _, cfg := range Builtins() {
		if cfg.Auth != "" {
			t.Fatalf("backend %s auth = %q, want empty", cfg.Name, cfg.Auth)
		}
	}

	t.Setenv("SKILLSMP_API_KEY", "skillsmp-token")
	t.Setenv("DECIMAL_API_KEY", "decimal-token")
	auth := map[string]string{}
	for _, cfg := range Builtins() {
		auth[cfg.Name] = cfg.Auth
	}
	if auth["skillsmp"] != "skillsmp-token" || auth["decimalai"] != "decimal-token" {
		t.Fatalf("auth = %v", auth)
	}
}
