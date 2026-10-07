package install

import "testing"

func TestSourceKind(t *testing.T) {
	cases := map[string]string{
		"owner/repo":                      "github",
		"https://github.com/owner/repo":   "github",
		"https://skills.sh/x/y/react":     "url",
		"https://app.decimal.ai/skills/x": "url",
		"/abs/path/skill":                 "local",
		"./relative":                      "local",
		"not a source!":                   "unknown",
		"":                                "unknown",
	}
	for source, want := range cases {
		if got := SourceKind(source); got != want {
			t.Fatalf("SourceKind(%q) = %q, want %q", source, got, want)
		}
	}
}
