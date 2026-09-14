package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a change set",
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{
				ChangeSetId: args[0],
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			cs := resp.ChangeSet
			return p.PrintOne(cs, changeSetID(cs), changeSetTable.Render(p, cs))
		},
	}
	return cmd
}
