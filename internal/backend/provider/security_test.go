package provider

import "testing"

func TestRawControlsCannotBecomeUsableIdentities(t *testing.T) {
	for _, raw := range []string{"acme/repo\n", "\nacme/repo", "acme/repo\t", "https://github.com/acme/repo/\n", "https://github.com/acme/repo\u0085"} {
		if got := RepoFromURL(raw); got != "" {
			t.Errorf("raw repository control laundered into %q", got)
		}
		if got := Source(raw); got != "" {
			t.Errorf("raw source control laundered into %q", got)
		}
	}
	for _, raw := range []string{"skill\n", "\tskill", "acme/\u0085skill", "skill\x1b"} {
		if got := Name(raw); got != "" {
			t.Errorf("raw name control laundered into %q", got)
		}
		if got := Slug(raw); got != "" {
			t.Errorf("raw slug control laundered into %q", got)
		}
	}
	for _, raw := range []string{"https://github.com/acme/repo%0a", "http://github.com/acme/repo", "https://user:password@github.com/acme/repo"} {
		if got := RepoFromURL(raw); got != "" {
			t.Errorf("unsafe URL canonicalized as repository %q", got)
		}
	}
	if got := Source(" https://example.org/skill "); got != "https://example.org/skill" {
		t.Fatalf("ordinary provider whitespace normalization changed: %q", got)
	}
}
