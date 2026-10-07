package prompt

import (
	"fmt"
	"go-s/internal/install"
	"go-s/internal/textsafe"
	"strings"

	huh "charm.land/huh/v2"
)

// PlanSummary renders an install plan as one confirm-prompt description:
// the exact per-source batches the install will execute, followed by the
// Install/Cancel choice. Pure: shared by the prompt and golden tests.
func PlanSummary(plan install.Plan) string {
	total := countSkills(plan)
	scope := plan.Scope
	if scope == "" {
		scope = "project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Install %d skill%s\n", total, plural(total))
	fmt.Fprintf(&b, "\n%s scope\n", strings.ToUpper(scope[:1])+scope[1:])
	fmt.Fprintf(&b, "Agents: %s\n", strings.Join(plan.Agents, ", "))
	for _, batch := range plan.Batches {
		fmt.Fprintf(&b, "\n%s\n", textsafe.Plain(batch.Source))
		for _, name := range batch.Skills {
			fmt.Fprintf(&b, "  • %s\n", textsafe.Plain(name))
		}
	}
	return b.String()
}

func countSkills(plan install.Plan) int {
	n := 0
	for _, batch := range plan.Batches {
		n += len(batch.Skills)
	}
	return n
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ConfirmPlan asks for install confirmation against the exact resulting
// plan. Abort counts as "no".
func ConfirmPlan(plan install.Plan, themeName string) (bool, error) {
	if !IsTTY() {
		return false, RequireTTY("install confirmation")
	}
	var ok bool
	width, _ := formBounds()
	if err := runForm(themeName, width, huh.NewGroup(
		huh.NewConfirm().
			Title("Install selected skills?").
			Description(PlanSummary(plan)).
			Affirmative("Install").
			Negative("Cancel").
			Value(&ok),
	)); err != nil {
		return false, err
	}
	return ok, nil
}
