package run

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newCancelCmd(opts *client.Options) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel an in-progress run",
		Long: `Cancel a run that is pending, queued, or running. Revisions that
have already succeeded remain applied. Pending and running revisions are
cancelled. If this run was blocking the queue, the next queued run is
automatically promoted.`,
		Example: `  admiral run cancel <run-id>
  admiral run cancel <run-id> --force`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Cancel run %s (revisions that already succeeded stay applied)", id)); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Run().CancelRun(cmd.Context(), &runv1.CancelRunRequest{
				RunId: id,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Run, RunID(resp.Run), RunTable.Render(p, resp.Run))
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
