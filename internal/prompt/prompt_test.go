package prompt

import (
	"errors"
	"testing"
)

func TestNonTTYPromptErrors(t *testing.T) {
	if IsTTY() {
		t.Skip("TTY attached")
	}
	if selected, err := SelectSkills(fixtureResult()); !errors.Is(err, ErrNonTTY) || selected != nil {
		t.Fatalf("SelectSkills: %v, %v", selected, err)
	}
	if err := RequireTTY("selection"); !errors.Is(err, ErrNonTTY) {
		t.Fatal(err)
	}
}
