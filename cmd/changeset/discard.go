package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newDiscardCmd(opts *client.Options) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "discard <change-set>",
		Short: "Discard a draft change set",
		Long: `Discard a draft change set.

The component names it reserved are released. A discarded change set
cannot be edited again.`,
		Example: `  admiral changeset discard cs-7f2a1c9d0e3b

  # Skip the confirmation prompt
  admiral changeset discard cs-7f2a1c9d0e3b --force`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			if err := input.RequireInteractiveOrForce(cmd, force); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			if err := input.Confirm(cmd, force, "Discard change set "+csID); err != nil {
				return err
			}
			if _, err := c.ChangeSet().DiscardChangeSet(cmd.Context(), &changesetv1.DiscardChangeSetRequest{
				ChangeSetId: csID,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "change set", csID, "discarded")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
