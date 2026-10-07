// Package agent renders advisory instruction briefs over shared scan results.
package agent

import (
	"embed"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/report"
)

const TemplateVersion = "1"

//go:embed instructions.md
var templates embed.FS

type Options struct {
	Bucket         string
	MaxSignals     int
	ContextLines   int
	NoInstructions bool
}
type Envelope struct {
	TemplateVersion string               `json:"templateVersion"`
	Signals         []model.MergedSignal `json:"signals"`
	Suggestions     []model.Suggestion   `json:"suggestions"`
	Unresolved      []model.Observation  `json:"unresolved"`
	Warnings        []string             `json:"warnings"`
	Instructions    string               `json:"instructions,omitempty"`
}

func Build(result model.ScanResult, opts Options) (Envelope, error) {
	if opts.Bucket == "" {
		opts.Bucket = "all"
	}
	if opts.Bucket != "all" && opts.Bucket != "suggested" {
		return Envelope{}, fmt.Errorf("bucket must be all or suggested")
	}
	if opts.MaxSignals < 0 || opts.ContextLines < 0 {
		return Envelope{}, fmt.Errorf("brief limits must be nonnegative")
	}
	e := Envelope{TemplateVersion: TemplateVersion, Signals: slices.Clone(result.Signals), Suggestions: []model.Suggestion{}, Unresolved: slices.Clone(result.Unresolved), Warnings: slices.Clone(result.Warnings)}
	slices.SortFunc(e.Signals, func(a, b model.MergedSignal) int {
		if a.Confidence > b.Confidence {
			return -1
		}
		if a.Confidence < b.Confidence {
			return 1
		}
		return strings.Compare(a.Domain+"\x00"+a.Key, b.Domain+"\x00"+b.Key)
	})
	if opts.MaxSignals > 0 && len(e.Signals) > opts.MaxSignals {
		e.Signals = e.Signals[:opts.MaxSignals]
	}
	for i := range e.Signals {
		if opts.ContextLines > 0 && len(e.Signals[i].Evidence) > opts.ContextLines {
			e.Signals[i].Evidence = slices.Clone(e.Signals[i].Evidence[:opts.ContextLines])
		}
	}
	for _, s := range result.Suggestions {
		if opts.Bucket == "all" || s.Bucket == "suggested" {
			e.Suggestions = append(e.Suggestions, s)
		}
	}
	if !opts.NoInstructions {
		b, err := templates.ReadFile("instructions.md")
		if err != nil {
			return Envelope{}, err
		}
		e.Instructions = string(b)
	}
	return e, nil
}

func JSON(w io.Writer, result model.ScanResult, opts Options) error {
	e, err := Build(result, opts)
	if err != nil {
		return err
	}
	return report.JSON(w, e)
}

// Markdown renders a concise assessment rather than requiring agents to parse
// a full scan dump. All workspace-controlled strings appear as inert code spans.
func Markdown(w io.Writer, result model.ScanResult, opts Options) error {
	e, err := Build(result, opts)
	if err != nil {
		return err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# sift assessment\n\nTemplate version: %s\n\n", TemplateVersion)

	counts := map[string]int{}
	for _, s := range e.Suggestions {
		counts[s.Bucket]++
	}
	workspace := "not provided"
	if result.Root != "" {
		workspace = inlineData(result.Root)
	}
	signalCount := fmt.Sprint(len(e.Signals))
	if len(e.Signals) != len(result.Signals) {
		signalCount = fmt.Sprintf("%d shown of %d", len(e.Signals), len(result.Signals))
	}
	fmt.Fprintf(&out, "## Summary\n\nWorkspace: %s\n\nSignals: %s | Suggested: %d | Possible: %d | External: %d | Hidden: %d | Unresolved: %d | Warnings: %d\n\nPlan only—nothing installed.\n\n",
		workspace, signalCount, counts["suggested"], counts["possible"], counts["external"], counts["hidden"], len(e.Unresolved), len(e.Warnings))

	out.WriteString("## Detected technologies\n\n")
	if len(e.Signals) == 0 {
		out.WriteString("None detected.\n\n")
	} else {
		for _, s := range e.Signals {
			fmt.Fprintf(&out, "- %s — confidence %.2f", inlineData(s.Key), s.Confidence)
			if s.Domain != "" {
				fmt.Fprintf(&out, "; domain %s", inlineData(s.Domain))
			}
			out.WriteByte('\n')
			writeDetails(&out, "Evidence", s.Evidence)
			writeDetails(&out, "Reasons", s.Reasons)
			writeDetails(&out, "Members", s.Members)
			if len(s.Layers) > 0 {
				layers := make([]string, len(s.Layers))
				for i, layer := range s.Layers {
					layers[i] = fmt.Sprint(layer)
				}
				fmt.Fprintf(&out, "  - Detector layers: %s\n", strings.Join(layers, ", "))
			}
			if s.RootObserved {
				out.WriteString("  - Also observed at workspace root\n")
			}
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Recommended skills\n\n")
	out.WriteString("Locally suggested candidates. Assess each before including it in a plan.\n\n")
	writeSuggestions(&out, e.Suggestions, "suggested")

	out.WriteString("## Other candidates\n\n")
	out.WriteString("Possible, external, and hidden candidates are **not** default recommendations. Possible and external candidates require explicit opt-in. External scores are provider rankings, not confidence or trust scores.\n\n")
	if counts["possible"]+counts["external"]+counts["hidden"] == 0 {
		out.WriteString("None.\n\n")
	} else {
		for _, bucket := range []string{"possible", "external", "hidden"} {
			if counts[bucket] == 0 {
				continue
			}
			fmt.Fprintf(&out, "### %s\n\n", strings.ToUpper(bucket[:1])+bucket[1:])
			writeSuggestions(&out, e.Suggestions, bucket)
		}
	}

	out.WriteString("## Unresolved findings\n\n")
	if len(e.Unresolved) == 0 {
		out.WriteString("None.\n\n")
	} else {
		for _, observation := range e.Unresolved {
			fmt.Fprintf(&out, "- %s — kind %s\n", inlineData(observation.Key), inlineData(string(observation.Kind)))
			writeDetail(&out, "Value", observation.Value)
			writeDetail(&out, "Member", observation.Member)
			writeDetail(&out, "Reason", observation.Reason)
			writeDetails(&out, "Evidence", observation.Evidence)
		}
		out.WriteByte('\n')
	}

	out.WriteString("## Warnings\n\n")
	if len(e.Warnings) == 0 {
		out.WriteString("None.\n\n")
	} else {
		for _, warning := range e.Warnings {
			fmt.Fprintf(&out, "- %s\n", inlineData(warning))
		}
		out.WriteByte('\n')
	}
	if e.Instructions != "" {
		fmt.Fprintf(&out, "## Assessment instructions\n\n%s", e.Instructions)
	}
	_, err = io.WriteString(w, out.String())
	return err
}

func writeSuggestions(out *strings.Builder, suggestions []model.Suggestion, bucket string) {
	count := 0
	for _, s := range suggestions {
		if s.Bucket != bucket {
			continue
		}
		count++
		fmt.Fprintf(out, "- %s / %s", inlineData(s.Skill.Source), inlineData(s.Skill.Name))
		if bucket == "external" {
			fmt.Fprintf(out, " — external rank %.2f (not a trust score)", s.ExternalScore)
		} else {
			fmt.Fprintf(out, " — confidence %.2f", s.Confidence)
		}
		out.WriteByte('\n')
		writeDetails(out, "Reasons", s.Reasons)
		writeDetails(out, "Evidence", s.Evidence)
		writeDetails(out, "Technologies", s.Technologies)
		writeDetails(out, "Members", s.Members)
		writeDetail(out, "Backend", s.SourceBackend)
		writeDetail(out, "URL", s.URL)
		if s.Stale {
			out.WriteString("  - Discovery result: stale cache\n")
		}
	}
	if count == 0 {
		out.WriteString("None.\n")
	}
	out.WriteByte('\n')
}

func writeDetail(out *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(out, "  - %s: %s\n", label, inlineData(value))
	}
}

func writeDetails(out *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		return
	}
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = inlineData(value)
	}
	fmt.Fprintf(out, "  - %s: %s\n", label, strings.Join(quoted, ", "))
}

// inlineData prevents untrusted paths, names, reasons and evidence from
// introducing headings, links or executable instructions into the Markdown.
// The delimiter is longer than every backtick run in the sanitized value.
func inlineData(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, report.Plain(value))
	maxRun, run := 0, 0
	for _, r := range value {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", maxRun+1)
	return fence + " " + value + " " + fence
}
