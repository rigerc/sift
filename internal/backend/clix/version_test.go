package clix

import (
	"context"
	"errors"
	"testing"
)

type fakeRunner struct {
	binary string
	args   []string
	stdout string
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, binary string, allowed []string, args []string) ([]byte, []byte, error) {
	f.binary, f.args = binary, args
	if f.err != nil {
		return nil, nil, f.err
	}
	return []byte(f.stdout), nil, nil
}

func TestPinnedAcceptsMatchingVersion(t *testing.T) {
	inner := &fakeRunner{stdout: "askill 0.1.15\n"}
	p := NewPinned(inner, "askill", "0.1.15")
	if _, _, err := p.Run(context.Background(), "askill", nil, []string{"find", "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inner.args) != 2 || inner.args[0] != "find" {
		t.Fatalf("expected passthrough call, got %v", inner.args)
	}
}

func TestPinnedFailsOnVersionDrift(t *testing.T) {
	p := NewPinned(&fakeRunner{stdout: "askill 0.2.0\n"}, "askill", "0.1.15")
	_, _, err := p.Run(context.Background(), "askill", nil, []string{"find", "x"})
	if CodeOf(err) != CodeVersionDrift {
		t.Fatalf("want version_drift, got %v", err)
	}
}

func TestPinnedFailsOnMissingVersion(t *testing.T) {
	p := NewPinned(&fakeRunner{stdout: "no version here\n"}, "askill", "0.1.15")
	_, _, err := p.Run(context.Background(), "askill", nil, []string{"find", "x"})
	if CodeOf(err) != CodeVersionDrift {
		t.Fatalf("want version_drift for missing version, got %v", err)
	}
	p = NewPinned(&fakeRunner{stdout: "no version here\n"}, "askill", "0.1.15")
	p.AllowMissing = true
	if _, _, err := p.Run(context.Background(), "askill", nil, []string{"find", "x"}); err != nil {
		t.Fatalf("unexpected error with AllowMissing: %v", err)
	}
}

func TestPinnedFailsWhenVersionCheckErrors(t *testing.T) {
	p := NewPinned(&fakeRunner{err: errors.New("spawn failed")}, "askill", "0.1.15")
	_, _, err := p.Run(context.Background(), "askill", nil, []string{"find", "x"})
	if CodeOf(err) != CodeUnknown {
		t.Fatalf("want unknown, got %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "askill version 0.1.15 (build 42)", want: "0.1.15"},
		{in: "4.11.1", want: "4.11.1"},
		{in: "v2.10.3", want: "2.10.3"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		if got := ParseVersion(tt.in); got != tt.want {
			t.Errorf("ParseVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
