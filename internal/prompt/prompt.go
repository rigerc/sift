// Package prompt provides standalone huh/v2 interactive prompts for the
// headless-first CLI. Every prompt runs as a blocking huh form directly on
// the terminal; nothing here starts a BubbleTea program or imports Cobra.
// The full-screen TUI under internal/ui remains available via `skillscan tui`.
package prompt

import (
	"errors"
	"fmt"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/ui/theme"
	"os"
	"strings"

	huh "charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// ErrNonTTY is returned when an interactive prompt is requested without a terminal.
var ErrNonTTY = errors.New("prompt: interactive terminal required")

// IsTTY reports whether the process has an interactive terminal attached.
func IsTTY() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// RequireTTY explains the headless alternatives when a prompt needs a terminal.
func RequireTTY(what string) error {
	return fmt.Errorf("%w for %s; use --json or --dry-run for headless output", ErrNonTTY, what)
}

func isAbort(err error) bool { return errors.Is(err, huh.ErrUserAborted) }

// OptionLabel renders one suggestion as a huh MultiSelect option title.
// The canonical (source, name) identity travels in the option value, so
// labels are display-only and never parsed back.
func OptionLabel(s model.Suggestion) string {
	score := s.Confidence
	if s.Bucket == "external" {
		score = s.ExternalScore
	}
	return fmt.Sprintf("[%s] %s (%s) %.2f", s.Bucket, s.Skill.Name, s.Skill.Source, score)
}

// Preselected returns the canonical keys pre-checked in the selection form:
// local suggested-bucket results only. External results always need explicit opt-in.
func Preselected(result model.ScanResult) []string {
	out := []string{}
	for _, s := range result.Suggestions {
		if s.Bucket == "suggested" {
			out = append(out, s.Skill.Key())
		}
	}
	return out
}

// Options builds huh options for every selectable suggestion in resolver order.
// Hidden-bucket results stay headless-only (same rule as report.Table).
// Values are canonical SkillRef keys; display order is restored by FilterSelected.
func Options(result model.ScanResult) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(result.Suggestions))
	for _, s := range result.Suggestions {
		if s.Bucket == "hidden" {
			continue
		}
		opts = append(opts, huh.NewOption(OptionLabel(s), s.Skill.Key()).Selected(s.Bucket == "suggested"))
	}
	return opts
}

// FilterSelected restores resolver order for the chosen canonical keys,
// dropping unknown or duplicated keys instead of failing.
func FilterSelected(result model.ScanResult, keys []string) []model.SkillRef {
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	out := []model.SkillRef{}
	seen := map[string]bool{}
	for _, s := range result.Suggestions {
		k := s.Skill.Key()
		if want[k] && !seen[k] {
			out = append(out, s.Skill)
			seen[k] = true
		}
	}
	return out
}

// SelectSkills runs a standalone huh MultiSelect over the scan suggestions.
// Abort (esc/ctrl-c) cancels silently with (nil, nil); empty results skip the form.
func SelectSkills(result model.ScanResult, themeName string) ([]model.SkillRef, error) {
	if !IsTTY() {
		return nil, RequireTTY("skill selection")
	}
	if len(result.Suggestions) == 0 {
		return nil, nil
	}
	var chosen []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Select skills to install").
			Description(fmt.Sprintf("Workspace: %s — suggested pre-selected, external needs explicit opt-in", result.Root)).
			Options(Options(result)...).
			Value(&chosen),
	)).WithTheme(theme.HuhTheme(themeName))
	if err := form.Run(); err != nil {
		if isAbort(err) {
			return nil, nil
		}
		return nil, err
	}
	return FilterSelected(result, chosen), nil
}

// PlanSummary renders an install plan as one confirm-prompt description.
// Pure: shared by the prompt and golden tests.
func PlanSummary(plan install.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d skill(s) from %d source(s) to %s scope (agents: %s):",
		countSkills(plan), len(plan.Batches), plan.Scope, strings.Join(plan.Agents, ", "))
	for _, batch := range plan.Batches {
		fmt.Fprintf(&b, "\n  %s: %s", batch.Source, strings.Join(batch.Skills, ", "))
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

// ConfirmPlan asks for install confirmation. Abort counts as "no".
func ConfirmPlan(plan install.Plan, themeName string) (bool, error) {
	if !IsTTY() {
		return false, RequireTTY("install confirmation")
	}
	var ok bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Install selected skills?").
			Description(PlanSummary(plan)).
			Affirmative("Install").
			Negative("Cancel").
			Value(&ok),
	)).WithTheme(theme.HuhTheme(themeName))
	if err := form.Run(); err != nil {
		if isAbort(err) {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}

// ShowWelcome presents the first-run note. Abort counts as acknowledged.
func ShowWelcome(themeName string) error {
	if !IsTTY() {
		return RequireTTY("welcome")
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().
			Title("Welcome to skillscan").
			Description("Scan a workspace for technologies and suggest installable agent skills.\n\nRun `skillscan scan` for the guided flow, `scan --json` for machine output, or `skillscan agent` for an AI-agent brief.").
			Next(true).
			NextLabel("Get started"),
	)).WithTheme(theme.HuhTheme(themeName))
	if err := form.Run(); err != nil {
		if isAbort(err) {
			return nil
		}
		return err
	}
	return nil
}
