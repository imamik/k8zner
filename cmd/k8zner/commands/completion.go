package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// Completion returns the completion command for shell autocompletion.
func Completion() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for k8zner.

To load completions:

Bash:
  $ source <(k8zner completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ k8zner completion bash > /etc/bash_completion.d/k8zner
  # macOS:
  $ k8zner completion bash > $(brew --prefix)/etc/bash_completion.d/k8zner

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. Execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ k8zner completion zsh > "${fpath[1]}/_k8zner"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ k8zner completion fish | source
  # To load completions for each session, execute once:
  $ k8zner completion fish > ~/.config/fish/completions/k8zner.fish

PowerShell:
  PS> k8zner completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> k8zner completion powershell > k8zner.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}
