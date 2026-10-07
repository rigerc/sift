package prompt

import (
	"fmt"
	"go-s/internal/model"
	"go-s/internal/textsafe"
	"go-s/internal/ui/theme"
	"math"
	"os"
	"strings"

	huh "charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// bucketLabels maps internal bucket names to their display form. External
// scores and local confidence are never rendered with the same vocabulary.
var bucketLabels = map[string]string{
	"suggested": "RECOMMENDED",
	"possible":  "POSSIBLE",
	"external":  "EXTERNAL",
}

// optionIndent aligns the detail line under the option name: the 11-char
// bucket column plus the 2-space gutter of the label format string.
const optionIndent = 13

// ScanCounts tallies scan suggestions by bucket for the summary header.
type ScanCounts struct {
	Suggested int
	Possible  int
	External  int
	Hidden    int
}

// CountSuggestions tallies the scan result by bucket.
func CountSuggestions(result model.ScanResult) ScanCounts {
	var c ScanCounts
	for _, s := range result.Suggestions {
		switch s.Bucket {
		case "suggested":
			c.Suggested++
		case "possible":
			c.Possible++
		case "external":
			c.External++
		case "hidden":
			c.Hidden++
		}
	}
	return c
}

// ScanSummary renders the orientation block shown before skill selection:
// workspace, detected technologies, bucket counts, and warnings. Evidence
// and full observation dumps stay in the headless table; this is navigation,
// not a report. Pure: shared by the prompt and unit tests.
func ScanSummary(result model.ScanResult) string {
	c := CountSuggestions(result)
	var b strings.Builder
	fmt.Fprintf(&b, "Workspace   %s\n", textsafe.Plain(result.Root))
	technology := "technology"
	if n := len(result.Signals); n != 1 {
		technology = "technologies"
	}
	fmt.Fprintf(&b, "Detected    %d %s\n", len(result.Signals), technology)
	parts := []string{}
	if c.Suggested > 0 {
		parts = append(parts, fmt.Sprintf("%d recommended", c.Suggested))
	}
	if c.Possible > 0 {
		parts = append(parts, fmt.Sprintf("%d possible", c.Possible))
	}
	if c.External > 0 {
		parts = append(parts, fmt.Sprintf("%d external", c.External))
	}
	if len(parts) == 0 {
		b.WriteString("Skills      none\n")
	} else {
		fmt.Fprintf(&b, "Skills      %s\n", strings.Join(parts, " · "))
	}
	appendWarnings(&b, result.Warnings)
	return b.String()
}

const maxListedWarnings = 3

// appendWarnings lists up to maxListedWarnings warnings and collapses the
// remainder, so interactivity never silently drops backend problems.
func appendWarnings(b *strings.Builder, warnings []string) {
	n := len(warnings)
	if n == 0 {
		return
	}
	if n == 1 {
		b.WriteString("\nWarnings\n")
	} else {
		fmt.Fprintf(b, "\nWarnings: %d\n", n)
	}
	for _, w := range warnings[:min(n, maxListedWarnings)] {
		fmt.Fprintf(b, "• %s\n", textsafe.Plain(w))
	}
	if extra := n - maxListedWarnings; extra > 0 {
		fmt.Fprintf(b, "• %d more\n", extra)
	}
}

// OptionLabel renders one suggestion as a two-line huh MultiSelect option
// title. Line 1 carries bucket, skill name, and score; line 2 carries the
// source (plus backend for external results) and the primary reason. The
// pad width aligns the score column across all options of one result.
// All repository-controlled text is sanitized before reaching the terminal.
func OptionLabel(s model.Suggestion, pad int) string {
	bucket := bucketLabels[s.Bucket]
	name := textsafe.Plain(s.Skill.Name)
	var score string
	switch s.Bucket {
	case "external":
		score = fmt.Sprintf("score %.2f", s.ExternalScore)
		if s.Stale {
			score += " · stale"
		}
	default:
		score = fmt.Sprintf("%d%% confidence", int(math.Round(s.Confidence*100)))
	}
	label := fmt.Sprintf("%-11s  %-*s  %s\n", bucket, pad, name, score)

	source := textsafe.Plain(s.Skill.Source)
	parts := []string{}
	if s.Bucket == "external" && s.SourceBackend != "" && s.SourceBackend != s.Skill.Source {
		parts = append(parts, textsafe.Plain(s.SourceBackend))
	}
	parts = append(parts, source)
	if s.Bucket == "external" && s.URL != "" {
		parts = append(parts, textsafe.Plain(s.URL))
	}
	if reason := primaryReason(s); reason != "" {
		parts = append(parts, reason)
	}
	label += strings.Repeat(" ", optionIndent) + strings.Join(parts, " · ")
	return label
}

// primaryReason picks the single most useful context line: the first reason,
// falling back to the first evidence path.
func primaryReason(s model.Suggestion) string {
	for _, r := range s.Reasons {
		if r != "" {
			return textsafe.Plain(r)
		}
	}
	for _, e := range s.Evidence {
		if e != "" {
			return textsafe.Plain(e)
		}
	}
	return ""
}

// optionSpec is the UI-independent shape of one selectable row: label, the
// canonical SkillRef key it submits, and whether it starts preselected.
type optionSpec struct {
	Label   string
	Key     string
	Checked bool
}

// optionSpecFor maps one suggestion to its option spec. Only suggested-bucket
// rows start checked; external results need explicit opt-in.
func optionSpecFor(s model.Suggestion, pad int) optionSpec {
	return optionSpec{Label: OptionLabel(s, pad), Key: s.Skill.Key(), Checked: s.Bucket == "suggested"}
}

// Options builds huh options for every selectable suggestion in resolver
// order, with the score column aligned to the widest skill name.
// Hidden-bucket results stay headless-only (same rule as report.Table).
// Values are canonical SkillRef keys; display order is restored by FilterSelected.
func Options(result model.ScanResult) []huh.Option[string] {
	widest := 0
	selectable := make([]model.Suggestion, 0, len(result.Suggestions))
	for _, s := range result.Suggestions {
		if s.Bucket == "hidden" {
			continue
		}
		if n := len(s.Skill.Name); n > widest {
			widest = n
		}
		selectable = append(selectable, s)
	}
	opts := make([]huh.Option[string], 0, len(selectable))
	for _, s := range selectable {
		spec := optionSpecFor(s, widest)
		opts = append(opts, huh.NewOption(spec.Label, spec.Key).Selected(spec.Checked))
	}
	return opts
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

const (
	maxFormWidth   = 100
	fallbackWidth  = 80
	fallbackHeight = 24
	uiChromeRows   = 10 // rows reserved for titles, descriptions, and help
	minAvailRows   = 6
)

// formBounds returns terminal-aware form dimensions: width capped for
// readability, and the rows still available for fields after the form
// chrome. Safe fallbacks apply when the terminal size cannot be determined.
func formBounds() (width, availRows int) {
	width, height := fallbackWidth, fallbackHeight
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 && h > 0 {
		width, height = w, h
	}
	width = min(width, maxFormWidth)
	availRows = max(height-uiChromeRows, minAvailRows)
	return width, availRows
}

// selectorHeight sizes the MultiSelect field. Small result sets render
// tightly; large ones fill the available rows and scroll in huh's viewport.
// The height must go on the field, not the form: huh's group-level viewport
// misjudges multiline option views and collapses the field viewport.
func selectorHeight(opts []huh.Option[string], availRows int) int {
	lines := 0
	for _, o := range opts {
		lines += strings.Count(o.Key, "\n") + 1
	}
	want := lines + 3 // title, description, and cursor line
	if want > availRows {
		want = availRows
	}
	return want
}

// SelectSkills runs the guided scan flow: a summary note for orientation,
// then a terminal-aware MultiSelect over the suggestions. The MultiSelect is
// filterable ("/") and supports ctrl+a / ctrl+e select-all/none via huh.
// Abort (esc/ctrl-c) cancels silently with (nil, nil); results without
// selectable skills stop after the summary instead of an empty selector.
func SelectSkills(result model.ScanResult, themeName string) ([]model.SkillRef, error) {
	if !IsTTY() {
		return nil, RequireTTY("skill selection")
	}
	width, availRows := formBounds()
	summary := ScanSummary(result)
	opts := Options(result)
	if len(opts) == 0 {
		if err := runForm(themeName, width, huh.NewGroup(
			huh.NewNote().Title("Scan complete").Description(summary+"\nNo installable skills found.").Next(true).NextLabel("Close"),
		)); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var chosen []string
	selector := huh.NewMultiSelect[string]().
		Title("Select skills to install").
		Description("space/x toggle · / filter · ctrl+a select all — recommended pre-selected, external needs opt-in.").
		Options(opts...).
		Height(selectorHeight(opts, availRows)).
		Value(&chosen)
	if err := runForm(themeName, width,
		huh.NewGroup(huh.NewNote().Title("Scan complete").Description(summary).Next(true).NextLabel("Continue")),
		huh.NewGroup(selector),
	); err != nil {
		return nil, err
	}
	return FilterSelected(result, chosen), nil
}

// runForm executes one blocking huh form with the app theme and a
// terminal-aware width. Abort (esc/ctrl-c) counts as cancel.
func runForm(themeName string, width int, groups ...*huh.Group) error {
	form := huh.NewForm(groups...).
		WithTheme(theme.HuhTheme(themeName)).
		WithWidth(width)
	if err := form.Run(); err != nil {
		if isAbort(err) {
			return nil
		}
		return err
	}
	return nil
}
