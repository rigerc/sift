package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rigerc/sift/internal/model"
)

func press(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }
func letter(r rune) tea.KeyPressMsg   { return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}) }

func selectorOf(t *testing.T, form *huh.Form) *skillSelector {
	t.Helper()
	form.NextGroup()
	field, ok := form.GetFocusedField().(*skillSelector)
	if !ok {
		t.Fatalf("not a selector: %T", form.GetFocusedField())
	}
	field.Focus()
	return field
}

func TestCancellationNeverReturnsPartialSelection(t *testing.T) {
	for _, abort := range []tea.KeyPressMsg{press(tea.KeyEscape), tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})} {
		got, err := selectWithRunner(fixtureResult(), 80, 14, func(form *huh.Form) error {
			selector := selectorOf(t, form)
			selector.Update(press(tea.KeyDown))
			selector.Update(press(tea.KeySpace))
			if len(selector.GetValue().([]string)) != 2 {
				t.Fatal("test did not create a partial selection")
			}
			form.Update(abort)
			if form.State != huh.StateAborted {
				t.Fatalf("cancel key did not abort: %v", abort)
			}
			return huh.ErrUserAborted
		})
		if err != nil || got != nil {
			t.Fatalf("partial result escaped: %#v, %v", got, err)
		}
	}
	failure := errors.New("terminal failure")
	got, err := selectWithRunner(fixtureResult(), 80, 14, func(*huh.Form) error { return failure })
	if !errors.Is(err, failure) || got != nil {
		t.Fatalf("failure was swallowed: %v %v", got, err)
	}
	got, err = selectWithRunner(fixtureResult(), 80, 14, func(*huh.Form) error { return context.Canceled })
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("context cancellation leaked result: %v %v", got, err)
	}
}

func TestFilterSelectsExternalWithoutLosingRecommended(t *testing.T) {
	got, err := selectWithRunner(fixtureResult(), 60, 14, func(form *huh.Form) error {
		selector := selectorOf(t, form)
		selector.Update(letter('/'))
		for _, r := range "three" {
			selector.Update(letter(r))
		}
		if hovered, ok := selector.Hovered(); !ok || hovered != "other/repo\x00three" {
			t.Fatalf("filter not working: %q %v", hovered, ok)
		}
		selector.Update(press(tea.KeyEnter)) // Apply, do not submit.
		if form.State != huh.StateNormal {
			t.Fatal("applying filter submitted the plan")
		}
		selector.Update(press(tea.KeySpace))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.SkillRef{{Source: "org/local", Name: "one"}, {Source: "other/repo", Name: "three"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filter lost selections/order: %#v", got)
	}
}

func TestEmptyFilterNavigationAndEscape(t *testing.T) {
	got, err := selectWithRunner(fixtureResult(), 60, 14, func(form *huh.Form) error {
		selector := selectorOf(t, form)
		selector.Update(letter('/'))
		for _, r := range "zzzz" {
			selector.Update(letter(r))
		}
		if _, ok := selector.Hovered(); ok {
			t.Fatal("unexpected filter match")
		}
		// Previously Huh could acquire cursor -1 and panic after recovering
		// the filter. Navigation with zero matches must be harmless.
		for _, key := range []rune{tea.KeyDown, tea.KeyEnd, tea.KeyUp, tea.KeyHome} {
			selector.Update(press(key))
		}
		form.Update(press(tea.KeyEscape))
		if form.State != huh.StateAborted {
			t.Fatal("Escape only cleared filter instead of aborting")
		}
		return huh.ErrUserAborted
	})
	if err != nil || got != nil {
		t.Fatalf("empty-filter abort: %#v %v", got, err)
	}
}

func TestBackendResultsCannotBePreselected(t *testing.T) {
	res := fixtureResult()
	res.Suggestions[0].SourceBackend = "search-provider"
	if len(Preselected(res)) != 0 {
		t.Fatal("backend result was preselected")
	}
	if optionSpecFor(res.Suggestions[0], 5).Checked {
		t.Fatal("backend checkbox was preselected")
	}
}

func TestNoSelectedAndNoSelectableSkills(t *testing.T) {
	got, err := selectWithRunner(fixtureResult(), 80, 14, func(form *huh.Form) error {
		selector := selectorOf(t, form)
		selector.Update(press(tea.KeySpace)) // Uncheck the sole recommended row.
		return nil
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("empty selection: %#v %v", got, err)
	}
	res := model.ScanResult{ResolveResult: model.ResolveResult{Suggestions: []model.Suggestion{{Skill: model.SkillRef{Source: "o/r", Name: "hidden"}, Bucket: "hidden"}}}}
	got, err = selectWithRunner(res, 60, 14, func(form *huh.Form) error {
		if _, ok := form.GetFocusedField().(*huh.Note); !ok {
			t.Fatalf("empty results got selector: %T", form.GetFocusedField())
		}
		return nil
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("no selectable: %#v %v", got, err)
	}
	if got := FilterSelected(res, []string{"o/r\x00hidden"}); len(got) != 0 {
		t.Fatal("hidden reference selected")
	}
}

func TestBoundedLayoutAndScrolling(t *testing.T) {
	for _, width := range []int{80, 60} {
		t.Run(fmt.Sprintf("%dx24", width), func(t *testing.T) {
			res := fixtureResult()
			res.Root = "/" + strings.Repeat("long/", 100)
			res.Warnings = []string{strings.Repeat("warning", 200), "two", "three", "four"}
			for i := 0; i < 40; i++ {
				res.Suggestions = append(res.Suggestions, model.Suggestion{Skill: model.SkillRef{Source: strings.Repeat("source/", 100), Name: fmt.Sprintf("skill-%02d-%s", i, strings.Repeat("界", 100))}, Bucket: "possible"})
			}
			w, rows := boundsForSize(width, 24)
			chosen := Preselected(res)
			form := selectionForm(res, w, rows, &chosen)
			form.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			assertBounded := func() {
				view := ansi.Strip(form.View())
				if n := strings.Count(view, "\n") + 1; n > 24 {
					t.Fatalf("%d rows: %s", n, view)
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatalf("overflow: %d > %d: %q", ansi.StringWidth(line), width, line)
					}
				}
			}
			assertBounded()
			selector := selectorOf(t, form)
			form.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			assertBounded()
			for i := 0; i < 45; i++ {
				form.Update(press(tea.KeyDown))
			}
			if hovered, ok := selector.Hovered(); !ok || !strings.Contains(hovered, "skill-39") {
				t.Fatalf("could not scroll to last row: %q %v", hovered, ok)
			}
			assertBounded()
		})
	}
}

func TestInlineDefaultThemeAndNoColorView(t *testing.T) {
	v := tea.NewView("\x1b[31mcolored\x1b[0m")
	v.AltScreen = true
	plain := inlineView(true)(v)
	if plain.AltScreen || strings.Contains(plain.Content, "\x1b") || plain.Content != "colored" {
		t.Fatalf("bad no-color inline view: %+v", plain)
	}
	styled := inlineView(false)(v)
	if styled.AltScreen || styled.Content != v.Content {
		t.Fatal("default styling altered")
	}
	// A fresh form renders identically to built-in Huh appearance. We don't
	// install a custom theme or global color profile for later forms.
	res := fixtureResult()
	var chosen []string
	form := selectionForm(res, 80, 14, &chosen)
	form.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(ansi.Strip(form.View()), "Scan complete") {
		t.Fatal("missing inline summary")
	}
	selectorOf(t, form)
	form.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(ansi.Strip(form.View()), "Select skills for an installation plan") {
		t.Fatal("selector does not explain its plan-only purpose")
	}
}

func TestRealHuhCancellationRestoresInlineRenderer(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, input := range []string{"\x03", "\x1b"} {
		var chrome bytes.Buffer
		var chosen []string
		reader, writer := io.Pipe()
		defer func() { _ = reader.Close() }()
		defer func() { _ = writer.Close() }()
		ready := make(chan struct{})
		var once sync.Once
		form := selectionForm(fixtureResult(), 60, 14, &chosen).
			WithProgramOptions(tea.WithWindowSize(60, 24), tea.WithoutSignalHandler()).
			WithInput(reader).WithOutput(&chrome).
			WithViewHook(func(v tea.View) tea.View {
				once.Do(func() { close(ready) })
				return inlineView(true)(v)
			})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		go func() {
			select {
			case <-ready:
			case <-ctx.Done():
				return
			}
			// Allow the first frame to flush before sending cancellation.
			time.Sleep(50 * time.Millisecond)
			_, _ = io.WriteString(writer, input)
		}()
		err := form.RunWithContext(ctx)
		cancel()
		if !errors.Is(err, huh.ErrUserAborted) {
			t.Fatalf("real cancellation %q: %v", input, err)
		}
		out := chrome.String()
		if strings.Contains(out, "\x1b[?1049h") || strings.Contains(out, "\x1b[?47h") {
			t.Fatalf("entered alternate screen: %q", out)
		}
		if !strings.Contains(out, "\x1b[?25h") {
			t.Fatalf("cursor was not restored: %q", out)
		}
		if strings.Contains(out, "Plan only") || strings.Contains(out, "npx ") {
			t.Fatal("chrome emitted executable instructions")
		}
	}
}
