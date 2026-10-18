package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type ChangeSetCmd struct {
	Cmd *cobra.Command
}

// NewChangeSetCmd is the changeset noun: a proposal against the components
// of one environment. Every verb that changes one cuts exactly one revision.
func NewChangeSetCmd(opts *client.Options) *ChangeSetCmd {
	root := &ChangeSetCmd{}

	cmd := &cobra.Command{
		Use:   "changeset",
		Short: "Propose changes to an environment's components",
		Long: `Propose changes to an environment's components.

A changeset is a proposal against one environment: add a component from
the registry, change its version, set or unset values, remove it. Each
command cuts one immutable revision. 'plan' starts a run that plans the
head and shows its diff; 'approve' and 'apply' act on that plan. 'revert'
opens a changeset that undoes an applied one.

A changeset is named by its ID (cs-7f2a1c9d0e3b) or by its environment
(shop/dev) while that environment has one open changeset. With several,
an environment names none of them and the command lists them instead.`,
		Aliases:       []string{"cs", "changesets"},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDiscardCmd(opts),
		newRevertCmd(opts),
		newAddCmd(opts),
		newSetCmd(opts),
		newUnsetCmd(opts),
		newRemoveCmd(opts),
		newValuesCmd(opts),
		newApproveCmd(opts),
		newApplyCmd(opts),
		newDiffCmd(opts),
		newPlanCmd(opts),
	)

	root.Cmd = cmd
	return root
}
