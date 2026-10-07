// Package textsafe removes terminal control characters from untrusted
// repository-controlled strings before they reach tabular or interactive
// output.
package textsafe

import (
	"strings"
	"unicode"
)

// Plain replaces control characters (escapes, newlines, tabs, C0/C1) with
// spaces so strings flowing into a report table or a huh form cannot inject
// terminal control sequences. Printable Unicode is preserved.
func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
