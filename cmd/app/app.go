package app

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type AppCmd struct {
	Cmd *cobra.Command
}

func NewAppCmd(opts *client.Options) *AppCmd {
	root := &AppCmd{}

	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
		Long: `Manage applications. An application is the top-level unit of ownership:
it holds environments, and each environment holds the components, variables,
and runs deployed there.

Commands accept an application name or ID as the positional argument.`,
		Aliases:       []string{"application"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newListCmd(opts),
		newCreateCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}
