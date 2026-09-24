package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type ChangeSetCmd struct {
	Cmd *cobra.Command
}

// NewChangeSetCmd is the change set noun: a proposal against the components
// of one environment. Every verb that changes one cuts exactly one revision.
func NewChangeSetCmd(opts *client.Options) *ChangeSetCmd {
	root := &ChangeSetCmd{}

	cmd := &cobra.Command{
		Use:   "changeset",
		Short: "Propose changes to an environment's components",
		Long: `Propose changes to an environment's components.

A change set is a draft against one environment: add a component from the
registry, move its pin, set or unset values, remove it. Each command cuts one
immutable revision. Nothing here plans or applies.

Change sets are addressed by ID (cs-7f2a1c9d0e3b).`,
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
	)

	root.Cmd = cmd
	return root
}
