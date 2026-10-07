// Package cmd provides the scanner and plan-only CLI.
package cmd

import (
	"context"

	"go-s/config"

	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	runtimeState   *RuntimeState
	processContext = context.Background()
)

type RuntimeState struct {
	Context context.Context
	Config  *config.EffectiveConfig
}

func SetContext(ctx context.Context) { processContext = ctx }
func Runtime() *RuntimeState         { return runtimeState }

var rootCmd = newRootCommand()

func newRootCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "skillscan",
		Short: "Scan workspaces and suggest agent skills",
		Long: `skillscan scans workspaces for technologies and suggests agent skills.
It generates installation plans and copyable commands; nothing is installed.
Run printed npx skills commands yourself after reviewing the skills.

On a terminal, scan offers bounded inline skill selection. Piped scans print a
table. Use --json for machine output or --dry-run for a recommended-local plan.`,
		Example: `  skillscan scan .
  skillscan scan --online=false
  skillscan scan --json
  skillscan scan --dry-run
  skillscan plan owner/repo --skill my-skill --agent claude-code
  skillscan plan owner/repo --skill my-skill --json
  skillscan agent .
  skillscan backends list
  skillscan version`,
		Version:       "1.0.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          func(c *cobra.Command, _ []string) error { return c.Help() },
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			effective, err := config.LoadEffective(GetConfigFile(), cfgFile != "", config.RuntimeOverrides{})
			if err != nil {
				return err
			}
			runtimeState = &RuntimeState{Context: c.Context(), Config: effective}
			return nil
		},
	}
	c.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Read configuration (default: $XDG_CONFIG_HOME/a-go-s/config.json)")
	return c
}

func Execute() error {
	runtimeState = nil
	defer func() { runtimeState = nil }()
	return rootCmd.ExecuteContext(processContext)
}

func GetRootCmd() *cobra.Command { return rootCmd }

func GetConfigFile() string {
	if cfgFile != "" {
		return cfgFile
	}
	return config.DefaultConfigPath()
}
