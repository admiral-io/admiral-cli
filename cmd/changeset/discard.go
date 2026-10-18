package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newDiscardCmd(opts *client.Options) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:               "discard <changeset>",
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		Short:             "Discard an open changeset",
		Long: `Discard an open changeset.

The component names it reserved are released and it is closed. A closed
changeset cannot be edited again.`,
		Example: `  admiral changeset discard cs-7f2a1c9d0e3b

  # Skip the confirmation prompt
  admiral changeset discard cs-7f2a1c9d0e3b --yes`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}

			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			if err := input.Confirm(cmd, yes, "Discard changeset "+csID); err != nil {
				return err
			}
			if _, err := c.ChangeSet().DiscardChangeSet(cmd.Context(), &changesetv1.DiscardChangeSetRequest{
				ChangeSetId: csID,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "changeset", csID, "discarded")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")

	return cmd
}
