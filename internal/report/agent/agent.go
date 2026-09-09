// Package agent renders advisory instruction briefs over shared scan results.
package agent

import (
	"embed"
	"fmt"
	"go-s/internal/model"
	"go-s/internal/report"
	"io"
	"slices"
	"strings"
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

func Markdown(w io.Writer, result model.ScanResult, opts Options) error {
	e, err := Build(result, opts)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "# skillscan assessment\n\nTemplate version: %s\n\n", TemplateVersion); err != nil {
		return err
	}
	// JSON-quoted data in a fence longer than any input backtick run prevents
	// repository strings from closing the data block and posing as instructions.
	var data strings.Builder
	if err := report.JSON(&data, struct {
		Signals     []model.MergedSignal `json:"signals"`
		Suggestions []model.Suggestion   `json:"suggestions"`
		Unresolved  []model.Observation  `json:"unresolved"`
		Warnings    []string             `json:"warnings"`
	}{e.Signals, e.Suggestions, e.Unresolved, e.Warnings}); err != nil {
		return err
	}
	fence := "```"
	for strings.Contains(data.String(), fence) {
		fence += "`"
	}
	if _, err = fmt.Fprintf(w, "## Workspace assessment and suggestions\n\n%sjson\n%s%s\n\n", fence, data.String(), fence); err != nil {
		return err
	}
	if e.Instructions != "" {
		_, err = fmt.Fprintf(w, "## Assessment instructions\n\n%s", e.Instructions)
	}
	return err
}
