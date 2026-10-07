package skillsmp

import "testing"

func TestMappingDoesNotHideRawSourceControls(t *testing.T) {
	a := &Adapter{name: "skillsmp"}
	for _, item := range []skill{
		{Name: "helper", GithubURL: "https://github.com/acme/repo\n"},
		{Name: "helper", SkillURL: "https://skillsmp.com/skills/helper\n"},
		{Name: "helper\n", GithubURL: "https://github.com/acme/repo"},
	} {
		if got := a.mapResults([]skill{item}); len(got) != 0 {
			t.Errorf("controls laundered into identity: %+v", got)
		}
	}
}
