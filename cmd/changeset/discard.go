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
		Use:   "discard <id>",
		Short: "Discard an OPEN change set",
		Long:  `Mark an OPEN change set as DISCARDED. Has no side effects on the environment; the change set record is retained for audit.`,
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if err := input.Confirm(cmd, force, "Discard change set "+id); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().DiscardChangeSet(cmd.Context(), &changesetv1.DiscardChangeSetRequest{
				ChangeSetId: id,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.ChangeSet, changeSetID(resp.ChangeSet), changeSetTable.Render(p, resp.ChangeSet))
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}
