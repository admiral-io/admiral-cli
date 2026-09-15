package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type EnvCmd struct {
	Cmd *cobra.Command
}

func NewEnvCmd(opts *client.Options) *EnvCmd {
	root := &EnvCmd{}

	cmd := &cobra.Command{
		Use:           "env",
		Short:         "Manage environments",
		Long:          `Manage deployment environments within an application.`,
		Aliases:       []string{"environment", "environments"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}
