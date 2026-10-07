package prompt

import (
	"errors"
	"go-s/internal/install"
	"testing"
)

func TestNonTTYPromptErrors(t *testing.T) {
	t.Parallel()
	// These guards fail fast before touching the terminal, so they hold
	// in CI (non-TTY). On a developer TTY only assert the heads-up error path.
	if IsTTY() {
		t.Skip("TTY attached; interactive forms cannot be asserted here")
	}
	if _, err := SelectSkills(fixtureResult(), "default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("SelectSkills: got %v", err)
	}
	if _, err := ConfirmPlan(install.Plan{}, "default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("ConfirmPlan: got %v", err)
	}
	if err := ShowWelcome("default"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("ShowWelcome: got %v", err)
	}
	if err := RequireTTY("x"); !errors.Is(err, ErrNonTTY) {
		t.Fatalf("RequireTTY must wrap ErrNonTTY: %v", err)
	}
}
