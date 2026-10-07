package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultsAndLegacyKeys(t *testing.T) {
	for _, input := range []string{"{}", `{"scan":{"maxDepth":0},"ui":{"themeName":"old"},"debug":"invalid","logLevel":42,"app":{"name":"different"}}`} {
		got, err := LoadFromBytes([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, DefaultConfig()) {
			t.Fatalf("defaults not preserved: %+v", got)
		}
	}
	got, err := LoadFromBytes([]byte(`{"scan":{"catalog":"local.json","maxDepth":64,"online":false},"install":{"global":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scan.Catalog != "local.json" || got.Scan.MaxDepth != 64 || got.Scan.Online || !got.Install.Global {
		t.Fatalf("explicit values lost: %+v", got)
	}
}

func TestMalformedAndInvalidConfigs(t *testing.T) {
	for _, input := range []string{"", "null", "[]", "{", "{} {}", `{"scan":{"maxDepth":-1}}`, `{"scan":{"maxDepth":65}}`, `{"scan":{"maxDepth":"8"}}`, `{"scan":{"online":"false"}}`} {
		t.Run(input, func(t *testing.T) {
			if _, err := LoadFromBytes([]byte(input)); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("accepted %q: %v", input, err)
			}
		})
	}
}

func TestLoadReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"scan":{"online":false,"maxDepth":0},"install":{"global":false},"unknown":{"keep":"me"}}`)
	if err := os.WriteFile(path, data, 0o444); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scan.Online || got.Scan.MaxDepth != 8 || got.Install.Global {
		t.Fatalf("bad values: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatalf("configuration rewritten: %q, %v", after, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrConfigNotFound) {
		t.Fatal(err)
	}
}

func TestSiftPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	if got := DefaultConfigPath(); got != filepath.Join(dir, "sift", "config.json") {
		t.Fatal(got)
	}
	if got := DefaultCacheDir(); got != filepath.Join(dir, "sift", "registry") {
		t.Fatal(got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", dir)
	if got := DefaultConfigPath(); got != filepath.Join(dir, ".config", "sift", "config.json") {
		t.Fatal(got)
	}
	if got := DefaultCacheDir(); got != filepath.Join(dir, ".cache", "sift", "registry") {
		t.Fatal(got)
	}
}
