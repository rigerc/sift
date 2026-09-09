package clix

import (
	"context"
	"fmt"
	"regexp"
	"sync"
)

var versionPattern = regexp.MustCompile(`\d+(\.\d+)*`)

// Pinned wraps a Runner and fails loudly when the installed CLI version does
// not match the pinned one, instead of silently trusting a changed JSON shape.
// The check runs at most once per instance.
type Pinned struct {
	// Inner performs the actual process execution.
	Inner Runner
	// Binary is the CLI used for the version probe.
	Binary string
	// Want is the pinned version, e.g. "0.1.15".
	Want string
	// AllowMissing skips the drift failure when the CLI reports no version at
	// all; the first real call will still surface any breakage.
	AllowMissing bool

	once   sync.Once
	result error
}

func NewPinned(inner Runner, binary, want string) *Pinned {
	return &Pinned{Inner: inner, Binary: binary, Want: want}
}

func (p *Pinned) Run(ctx context.Context, binary string, allowed []string, args []string) ([]byte, []byte, error) {
	if err := p.verify(ctx); err != nil {
		return nil, nil, err
	}
	return p.Inner.Run(ctx, binary, allowed, args)
}

func (p *Pinned) verify(ctx context.Context) error {
	p.once.Do(func() {
		stdout, _, err := p.Inner.Run(ctx, p.Binary, []string{"--version"}, []string{"--version"})
		if err != nil {
			p.result = &Error{Code: CodeUnknown, Message: fmt.Sprintf("version check for %s failed: %s", p.Binary, err.Error())}
			return
		}
		got := ParseVersion(string(stdout))
		if got == "" {
			if p.AllowMissing {
				return
			}
			p.result = &Error{Code: CodeVersionDrift, Message: fmt.Sprintf("%s did not report a version (pinned %s)", p.Binary, p.Want)}
			return
		}
		if got != p.Want {
			p.result = &Error{Code: CodeVersionDrift, Message: fmt.Sprintf("%s version %s does not match pinned %s", p.Binary, got, p.Want)}
		}
	})
	return p.result
}

// ParseVersion extracts the first dotted-numeric token from version output.
func ParseVersion(out string) string {
	return versionPattern.FindString(out)
}
