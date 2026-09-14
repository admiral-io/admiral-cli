package run

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type RunCmd struct {
	Cmd *cobra.Command
}

func NewRunCmd(opts *client.Options) *RunCmd {
	root := &RunCmd{}

	cmd := &cobra.Command{
		Use:           "run",
		Short:         "Inspect runs",
		Long:          `Inspect runs (plan/apply executions). Mutations route through 'admiral changeset plan/apply'; this surface is observation + recovery only.`,
		Aliases:       []string{"runs"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newRollbackCmd(opts),
		newCancelCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newLogsCmd(opts),
	)

	root.Cmd = cmd
	return root
}
