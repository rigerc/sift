package screens

import (
	"context"
	"fmt"
	"go-s/internal/install"
	"go-s/internal/task"
	"go-s/internal/ui/theme"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Installed struct {
	theme.ThemeAware
	ctx       context.Context
	root      string
	services  Services
	width     int
	loading   bool
	statuses  []install.Status
	errorText string
}

func NewInstalled(ctx context.Context, root string, services Services) *Installed {
	return &Installed{ctx: ctx, root: root, services: services}
}
func (s *Installed) SetWidth(w int) Screen        { s.width = w; return s }
func (s *Installed) Cancel()                      {}
func (s *Installed) ApplyTheme(state theme.State) { s.ApplyThemeState(state) }
func (s *Installed) Init() tea.Cmd {
	if s.services.Status == nil {
		s.errorText = "status service unavailable"
		return nil
	}
	s.loading = true
	return task.Run(s.ctx, "status", func(context.Context) ([]install.Status, error) { return s.services.Status(s.root) })
}

func (s *Installed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case task.DoneMsg[[]install.Status]:
		if m.Label == "status" {
			s.loading = false
			s.statuses = m.Value
		}
	case task.ErrMsg:
		if m.Label == "status" {
			s.loading = false
			s.errorText = m.Err.Error()
		}
	case tea.KeyPressMsg:
		if m.String() == "esc" {
			return s, func() tea.Msg { return BackMsg{} }
		}
	}
	return s, nil
}
func (s *Installed) View() tea.View { return tea.NewView(s.Body()) }
func (s *Installed) Body() string {
	if s.loading {
		return "Loading installed skills…"
	}
	if s.errorText != "" {
		return "Error: " + s.errorText
	}
	rows := []string{"Installed skills", ""}
	for _, st := range s.statuses {
		rows = append(rows, fmt.Sprintf("%-10s %s/%s", st.Status, st.Source, st.Name))
	}
	if len(s.statuses) == 0 {
		rows = append(rows, "No tracked skills.")
	}
	rows = append(rows, "", "esc: back")
	return lipgloss.NewStyle().Width(s.width).Render(strings.Join(rows, "\n"))
}

// Install is the confirmation/progress screen for an explicit install plan.
type Install struct {
	theme.ThemeAware
	ctx       context.Context
	services  Services
	plan      install.Plan
	width     int
	started   bool
	loading   bool
	errorText string
}

func NewInstall(ctx context.Context, plan install.Plan, services Services) *Install {
	return &Install{ctx: ctx, plan: plan, services: services}
}
func (s *Install) SetWidth(w int) Screen        { s.width = w; return s }
func (s *Install) ApplyTheme(state theme.State) { s.ApplyThemeState(state) }
func (s *Install) Cancel()                      {}
func (s *Install) Init() tea.Cmd                { return nil }
func (s *Install) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		switch m.String() {
		case "esc":
			return s, func() tea.Msg { return BackMsg{} }
		case "y", "enter":
			if !s.started && s.services.Install != nil {
				s.started, s.loading = true, true
				return s, task.Run(s.ctx, "install-plan", func(ctx context.Context) (string, error) { return "", s.services.Install(ctx, s.plan) })
			}
		}
	case task.DoneMsg[string]:
		if m.Label == "install-plan" {
			s.loading = false
		}
	case task.ErrMsg:
		if m.Label == "install-plan" {
			s.loading = false
			s.errorText = m.Err.Error()
		}
	}
	return s, nil
}
func (s *Install) View() tea.View { return tea.NewView(s.Body()) }
func (s *Install) Body() string {
	if s.loading {
		return "Installing skills…"
	}
	if s.errorText != "" {
		return "Install failed: " + s.errorText
	}
	rows := []string{"Install plan", ""}
	for _, b := range s.plan.Batches {
		rows = append(rows, fmt.Sprintf("%s: %s", b.Source, strings.Join(b.Skills, ", ")))
	}
	rows = append(rows, "", "y/enter: confirm   esc: cancel")
	return lipgloss.NewStyle().Width(s.width).Render(strings.Join(rows, "\n"))
}
