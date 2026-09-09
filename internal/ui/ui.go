// Package ui provides the TUI entry point for go-s.
package ui

import (
	"context"
	"go-s/config"
	"go-s/internal/app"
	"go-s/internal/model"
	"go-s/internal/ui/screens"

	tea "charm.land/bubbletea/v2"
)

// New creates a new root model from the config.
// ctx and cancel are the application-wide context for graceful shutdown.
// configPath is the path to persist settings; empty means no file save.
// firstRun indicates that no config file existed before this launch.
func New(ctx context.Context, cancel context.CancelFunc, cfg config.Config, configPath string, firstRun bool, services ...screens.Services) rootModel {
	return NewWithPersisted(ctx, cancel, cfg, cfg, configPath, firstRun, services...)
}

// NewWithPersisted supplies the file-backed configuration separately so
// one-shot environment and CLI overrides are not written by settings.
func NewWithPersisted(ctx context.Context, cancel context.CancelFunc, cfg, persisted config.Config, configPath string, firstRun bool, services ...screens.Services) rootModel {
	return NewWithScreen(ctx, cancel, cfg, persisted, configPath, firstRun, screens.NewHome(), services...)
}

func NewWithScreen(ctx context.Context, cancel context.CancelFunc, cfg, persisted config.Config, configPath string, firstRun bool, initial screens.Screen, services ...screens.Services) rootModel {
	var deps screens.Services
	if len(services) > 0 {
		deps = services[0]
	}
	m := newRootModel(ctx, cancel, cfg, configPath, firstRun, deps)
	if initial != nil {
		m.current = initial
	}
	m.persistedCfg = persisted
	m.preserveRuntime = cfg.Debug != persisted.Debug || cfg.LogLevel != persisted.LogLevel
	return m
}

// ServicesFromApp adapts the UI-independent application service to screen
// callbacks. Scan options are captured per interactive session.
func ServicesFromApp(service app.Service, opts app.ScanOptions) screens.Services {
	return screens.Services{
		Scan: func(ctx context.Context, root string) (model.ScanResult, error) {
			return service.Scan(ctx, root, opts)
		},
		PlanInstall: service.PlanInstall,
		Install:     service.Install,
		Status:      service.Status,
	}
}

// Run starts the TUI program. ctx is used to cancel background goroutines on quit.
func Run(ctx context.Context, m rootModel) error {
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}
