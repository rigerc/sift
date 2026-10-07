package officialskills

import "testing"

func TestRegistryDoesNotHideRawRepositoryControls(t *testing.T) {
	for _, repo := range []string{"acme/repo\n", "\nacme/repo", "acme/repo\u0085"} {
		skill := Skill{Name: "helper", RepoPtr: &repo, URL: "https://github.com/acme/repo"}
		if skill.Repo() != "" || len(InstallableOnly([]Skill{skill})) != 0 {
			t.Errorf("raw repository controls normalized away: %+v", skill)
		}
	}
}
