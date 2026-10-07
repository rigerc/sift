// Package cmd provides the scanner and plan-only CLI.
package cmd

import (
	"context"

	"github.com/rigerc/sift/config"

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
		Use:   "sift",
		Short: "Scan workspaces and suggest agent skills",
		Long: `sift scans workspaces for technologies and suggests agent skills.
It generates installation plans and copyable commands; nothing is installed.
Run printed npx skills commands yourself after reviewing the skills.

On a terminal, scan offers bounded inline skill selection. Piped scans print a
table. Use --json for machine output or --dry-run for a recommended-local plan.`,
		Example: `  sift scan .
  sift scan --online=false
  sift scan --json
  sift scan --dry-run
  sift plan owner/repo --skill my-skill --agent claude-code
  sift plan owner/repo --skill my-skill --json
  sift agent .
  sift backends list
  sift version`,
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
		"Read configuration (default: $XDG_CONFIG_HOME/sift/config.json)")
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
