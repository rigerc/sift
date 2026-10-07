// Package report renders deterministic headless output.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/rigerc/sift/internal/model"
	"github.com/rigerc/sift/internal/textsafe"
)

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(true)
	return enc.Encode(v)
}

// Plain removes terminal control characters from untrusted repository strings.
// It delegates to the shared textsafe implementation so every renderer
// (tabular and interactive) sanitizes identically.
func Plain(s string) string {
	return textsafe.Plain(s)
}

func Table(w io.Writer, result model.ScanResult, verbose bool) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "BUCKET\tSOURCE\tSKILL\tCONFIDENCE / EXTERNAL SCORE\tURL\tREASON\tEVIDENCE"); err != nil {
		return err
	}
	for _, s := range result.Suggestions {
		if s.Bucket == "hidden" && !verbose {
			continue
		}
		score := fmt.Sprintf("%.2f", s.Confidence)
		if s.Bucket == "external" {
			score = fmt.Sprintf("external %.2f", s.ExternalScore)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.Bucket, Plain(s.Skill.Source), Plain(s.Skill.Name), score, Plain(s.URL), Plain(strings.Join(s.Reasons, "; ")), Plain(strings.Join(s.Evidence, ", "))); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(w, "Warning: %s\n", Plain(warning)); err != nil {
			return err
		}
	}
	return nil
}
