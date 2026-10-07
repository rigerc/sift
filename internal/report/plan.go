package report

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/rigerc/sift/internal/plan"
)

// PlanText renders an already validated plan without applying it. The upstream
// tool, not sift, resolves conflicts with existing installations.
func PlanText(w io.Writer, p plan.Plan) error {
	if strings.ContainsFunc(p.Root, unicode.IsControl) {
		return fmt.Errorf("workspace path contains terminal control characters")
	}
	var out strings.Builder
	count := 0
	for _, batch := range p.Batches {
		count += len(batch.Skills)
	}
	fmt.Fprintln(&out, "Plan only—nothing installed")
	fmt.Fprintf(&out, "Workspace: %s\nScope: %s\nAgents: %s\nSkills: %d\n", Plain(p.Root), Plain(p.Scope), Plain(strings.Join(p.Agents, ", ")), count)
	for _, batch := range p.Batches {
		fmt.Fprintf(&out, "  %s: %s\n", Plain(batch.Source), Plain(strings.Join(batch.Skills, ", ")))
	}
	if count > 0 {
		fmt.Fprintln(&out, "\nRun these commands yourself (POSIX shell):")
		for _, batch := range p.Batches {
			fmt.Fprintf(&out, "cd %s && npx %s\n", shellQuote(p.Root), shellJoin(batch.Argv))
		}
		fmt.Fprintln(&out, "\nStructural validation does not establish trust. Review skills before applying; the upstream tool handles existing-installation conflicts.")
	}
	_, err := io.WriteString(w, out.String())
	return err
}
