package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type ChangeSetCmd struct {
	Cmd *cobra.Command
}

func NewChangeSetCmd(opts *client.Options) *ChangeSetCmd {
	root := &ChangeSetCmd{}

	cmd := &cobra.Command{
		Use:           "changeset",
		Short:         "Manage change sets",
		Long:          `Create and manage change sets -- the unit of work for proposing component and variable changes against an application+environment.`,
		Aliases:       []string{"cs", "changesets"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newListCmd(opts),
		newDiscardCmd(opts),
		newCopyCmd(opts),
		newEntryCmd(opts),
		newVarCmd(opts),
		newPlanCmd(opts),
		newApplyCmd(opts),
		newDiffCmd(opts),
	)

	root.Cmd = cmd
	return root
}
