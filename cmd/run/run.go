// Package run is the admiral run command group, and the waiting and
// printing that changeset plan shares with it.
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
	cmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"runs"},
		Short:   "Inspect, approve and apply runs",
		Long: `A run plans a revision of a changeset against its environment, and once
approved applies it. 'admiral changeset plan' starts one.

Commands take a run ID (run-xxxxxxxxxxxx).`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(newListCmd(opts), newGetCmd(opts), newApproveCmd(opts), newApplyCmd(opts),
		newAcceptCmd(opts), newCancelCmd(opts), newDiffCmd(opts), newLogsCmd(opts))
	return &RunCmd{Cmd: cmd}
}
