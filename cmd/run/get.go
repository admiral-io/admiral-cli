package run

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <run-id>",
		Short:   "Get a run",
		Example: `  admiral run get <run-id>`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			runResp, err := c.Run().GetRun(cmd.Context(), &runv1.GetRunRequest{
				RunId: args[0],
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(runResp.Run, RunID(runResp.Run), RunTable.Render(p, runResp.Run))
		},
	}
	return cmd
}
