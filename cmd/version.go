// Package cmd provides the CLI commands for the application.
package cmd

import (
	"fmt"
	"go-s/config"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `All software has versions. This one is no exception.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.DefaultConfig()
		if cfgFile != "" {
			if fileCfg, err := config.Load(cfgFile); err == nil {
				cfg = fileCfg
			}
		}
		fmt.Printf("skillscan v%s\n", cfg.App.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
