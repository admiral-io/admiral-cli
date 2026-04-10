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
  $ source <(admctl completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ admctl completion bash > /etc/bash_completion.d/admctl
  # macOS:
  $ admctl completion bash > $(brew --prefix)/etc/bash_completion.d/admctl

Zsh:
  $ source <(admctl completion zsh)
  # To load completions for each session, execute once:
  $ admctl completion zsh > "${fpath[1]}/_admctl"

Fish:
  $ admctl completion fish | source
  # To load completions for each session, execute once:
  $ admctl completion fish > ~/.config/fish/completions/admctl.fish

PowerShell:
  PS> admctl completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> admctl completion powershell > admctl.ps1
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
