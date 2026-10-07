package textsafe

import "testing"

func TestPlainRemovesControlCharacters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"clean", "clean"},
		{"bad\x1b[2J\ntext", "bad [2J text"},
		{"tab\tsep", "tab sep"},
		{"cr\rline", "cr line"},
		{"bell\x07!", "bell !"},
		{"del\x7f?", "del ?"},
		{"c1\u009b3", "c1 3"},
		{"uni→çké", "uni→çké"},
	}
	for _, c := range cases {
		if got := Plain(c.in); got != c.want {
			t.Errorf("Plain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
