package skyll

import "testing"

func TestMappingDoesNotHideRawSourceControls(t *testing.T) {
	a := &Adapter{name: "skyll"}
	for _, skill := range []skyllSkill{
		{ID: "helper", Source: "acme/repo\n"},
		{ID: "helper", Refs: skyllRefs{GitHub: "https://github.com/acme/repo\n"}},
		{ID: "helper", Refs: skyllRefs{SkillsSh: "https://skyll.app/skills/helper\n"}},
	} {
		if got := a.mapResults([]skyllSkill{skill}); len(got) != 0 {
			t.Errorf("controls laundered into identity: %+v", got)
		}
	}
}
