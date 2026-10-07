package decimalai

import "testing"

func TestMappingDoesNotHideRawRepositoryControls(t *testing.T) {
	a := &Adapter{name: "decimalai"}
	got := a.mapResults([]skill{{URLSlug: "helper", GitHubURL: "https://github.com/acme/repo\n"}})
	if len(got) != 1 || got[0].Skill.Source != DetailBase+"/skills/helper" {
		t.Fatalf("unsafe GitHub value must not become a repository (safe provider fallback is independent): %+v", got)
	}
	if got := a.mapResults([]skill{{URLSlug: "helper\n", GitHubURL: "https://github.com/acme/repo"}}); len(got) != 0 {
		t.Fatalf("raw name controls normalized away: %+v", got)
	}
}
