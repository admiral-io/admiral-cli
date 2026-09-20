package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
)

// shells completion can generate a script for.
var shells = []string{"bash", "zsh", "fish", "powershell"}

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
		ValidArgs:             shells,
		Args:                  flags.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(out)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			}
			return cmderr.Usage("unknown shell %q; must be one of %s", args[0], strings.Join(shells, ", "))
		},
	}
	return cmd
}
