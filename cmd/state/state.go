package state

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type StateCmd struct {
	Cmd *cobra.Command
}

func NewStateCmd(opts *client.Options) *StateCmd {
	root := &StateCmd{}

	cmd := &cobra.Command{
		Use:           "state",
		Short:         "Manage Terraform state",
		Long:          `Pull and push Terraform state for infrastructure components.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newPullCmd(opts),
		newPushCmd(opts),
	)

	root.Cmd = cmd
	return root
}
