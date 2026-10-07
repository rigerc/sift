package screens

import (
	"context"
	"fmt"
	"go-s/internal/install"
	"go-s/internal/model"
	"go-s/internal/task"
	"go-s/internal/ui/modal"
	"go-s/internal/ui/theme"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Services is the UI adapter boundary. The command/runtime layer supplies
// these callbacks; screens never import Cobra or the application service.
type Services struct {
	Scan        func(context.Context, string) (model.ScanResult, error)
	PlanInstall func(string, model.ScanResult, []model.SkillRef, install.Options) (install.Plan, error)
	Install     func(context.Context, install.Plan) error
	Status      func(string) ([]install.Status, error)
}

type Scan struct {
	theme.ThemeAware
	ctx         context.Context
	services    Services
	root        string
	width       int
	operationID string
	cancel      context.CancelFunc
	loading     bool
	result      model.ScanResult
	selected    []bool
	cursor      int
	errorText   string
}

func NewScan(ctx context.Context, root string, services Services) *Scan {
	return &Scan{ctx: ctx, root: root, services: services}
}

func (s *Scan) SetWidth(w int) Screen { s.width = w; return s }
func (s *Scan) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *Scan) ApplyTheme(state theme.State) { s.ApplyThemeState(state) }

func (s *Scan) Init() tea.Cmd {
	if s.services.Scan == nil {
		s.errorText = "scan service unavailable"
		return nil
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancel = cancel
	s.loading = true
	s.operationID = fmt.Sprintf("scan-%d", time.Now().UnixNano())
	id := s.operationID
	return task.RunWithID(ctx, id, "scan", func(ctx context.Context) (model.ScanResult, error) {
		return s.services.Scan(ctx, s.root)
	})
}

func (s *Scan) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case task.DoneMsg[model.ScanResult]:
		if m.Label != "scan" || m.OperationID != s.operationID {
			return s, nil
		}
		s.loading = false
		s.result = m.Value
		s.selected = make([]bool, len(s.result.Suggestions))
		for i, suggestion := range s.result.Suggestions {
			s.selected[i] = suggestion.Bucket == "suggested"
		}
		return s, nil
	case task.DoneMsg[string]:
		if m.Label == "install" && m.OperationID == s.operationID {
			s.loading = false
		}
		return s, nil
	case task.ErrMsg:
		if (m.Label != "scan" && m.Label != "install") || m.OperationID != s.operationID {
			return s, nil
		}
		s.loading = false
		if m.Err != context.Canceled {
			s.errorText = m.Err.Error()
		}
		return s, nil
	case modal.ConfirmedMsg:
		if m.ID != "scan-install" {
			return s, nil
		}
		return s.startInstall()
	case tea.KeyPressMsg:
		switch m.String() {
		case "esc":
			if s.cancel != nil {
				s.cancel()
			}
			return s, func() tea.Msg { return BackMsg{} }
		case "up", "k", "down", "j":
			s.toggleNearest(m.String())
			return s, nil
		case " ":
			s.toggleNearest(" ")
			return s, nil
		case "i", "enter":
			if len(s.result.Suggestions) == 0 {
				return s, nil
			}
			return s, modal.ShowConfirm("scan-install", "Install selected skills?", fmt.Sprintf("%d skills selected", s.countSelected()))
		}
	}
	return s, nil
}

func (s *Scan) toggleNearest(direction string) {
	if len(s.selected) == 0 {
		return
	}
	idx := s.cursor
	if direction == "down" || direction == "j" {
		idx = (idx + 1) % len(s.selected)
	}
	if direction == "up" || direction == "k" {
		idx = (idx + len(s.selected) - 1) % len(s.selected)
	}
	s.cursor = idx
	if direction == " " {
		s.selected[idx] = !s.selected[idx]
	}
}

func (s *Scan) countSelected() int {
	n := 0
	for _, v := range s.selected {
		if v {
			n++
		}
	}
	return n
}

func (s *Scan) startInstall() (tea.Model, tea.Cmd) {
	if s.services.PlanInstall == nil || s.services.Install == nil {
		s.errorText = "install service unavailable"
		return s, nil
	}
	refs := make([]model.SkillRef, 0)
	for i, selected := range s.selected {
		if selected {
			refs = append(refs, s.result.Suggestions[i].Skill)
		}
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancel = cancel
	s.loading = true
	s.operationID = fmt.Sprintf("install-%d", time.Now().UnixNano())
	id := s.operationID
	return s, task.RunWithID(ctx, id, "install", func(ctx context.Context) (string, error) {
		plan, err := s.services.PlanInstall(s.root, s.result, refs, install.Options{})
		if err != nil {
			return "", err
		}
		return "", s.services.Install(ctx, plan)
	})
}

func (s *Scan) View() tea.View { return tea.NewView(s.Body()) }
func (s *Scan) Body() string {
	if s.loading {
		return lipgloss.NewStyle().Foreground(s.Palette().Primary).Render("Scanning…")
	}
	if s.errorText != "" {
		return lipgloss.NewStyle().Foreground(s.Palette().Error).Render("Error: " + s.errorText)
	}
	rows := []string{"Scan results", "", fmt.Sprintf("Workspace: %s", s.root)}
	for i, v := range s.result.Suggestions {
		mark := "[ ]"
		if s.selected[i] {
			mark = "[x]"
		}
		cursor := " "
		if s.cursor == i {
			cursor = ">"
		}
		score := v.Confidence
		if v.Bucket == "external" {
			score = v.ExternalScore
		}
		rows = append(rows, fmt.Sprintf("%s%s %-14s %s (%.2f)", cursor, mark, v.Bucket, v.Skill.Name, score))
		if v.URL != "" {
			rows = append(rows, "    "+v.URL)
		}
		if len(v.Reasons) > 0 {
			rows = append(rows, "    "+v.Reasons[0])
		}
	}
	if len(s.result.Suggestions) == 0 {
		rows = append(rows, "No local suggestions found.")
	}
	rows = append(rows, "", "space/up/down: select   i/enter: install   esc: back")
	return lipgloss.NewStyle().Width(s.width).Render(strings.Join(rows, "\n"))
}

func (s *Scan) ShortHelp() []key.Binding { return nil }
