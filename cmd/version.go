package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Args:  cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(c.OutOrStdout(), "skillscan v%s\n", c.Root().Version)
		return err
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
