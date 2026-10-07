// Package cmd provides the CLI commands for the application.
package cmd

import (
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:
  $ source <(sift completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ sift completion bash > /etc/bash_completion.d/sift
  # macOS:
  $ sift completion bash > /usr/local/etc/bash_completion.d/sift

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ sift completion zsh > "${fpath[1]}/_sift"

  # You will need to start a new shell for this setup to take effect.

fish:
  $ sift completion fish | source

  # To load completions for each session, execute once:
  $ sift completion fish > ~/.config/fish/completions/sift.fish

PowerShell:
  PS> sift completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> sift completion powershell > sift.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			_ = cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			_ = cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			_ = cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
