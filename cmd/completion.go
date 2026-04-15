package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for Admiral CLI.

To load completions:

Bash:
  $ source <(admiral completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ admiral completion bash > /etc/bash_completion.d/admiral
  # macOS:
  $ admiral completion bash > $(brew --prefix)/etc/bash_completion.d/admiral

Zsh:
  $ source <(admiral completion zsh)
  # To load completions for each session, execute once:
  $ admiral completion zsh > "${fpath[1]}/_admiral"

Fish:
  $ admiral completion fish | source
  # To load completions for each session, execute once:
  $ admiral completion fish > ~/.config/fish/completions/admiral.fish

PowerShell:
  PS> admiral completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> admiral completion powershell > admiral.ps1
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.RangeArgs(0, 1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
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
