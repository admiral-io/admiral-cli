package app

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get an application",
		Long: `Print the one-line summary of an application. Use 'describe' for the full
view and '-o json' for the raw record.`,
		Example: `  # Get an application by name
  admiral app get billing-api

  # Get an application by ID
  admiral app get 550e8400-e29b-41d4-a716-446655440000

  # Raw record as JSON
  admiral app get billing-api -o json`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Apps(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			id, err := resolve.App(cmd.Context(), c.Application(), args[0])
			if err != nil {
				return err
			}

			resp, err := c.Application().GetApplication(cmd.Context(), &applicationv1.GetApplicationRequest{
				ApplicationId: id,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Application, resp.Application.Name, appTable.Render(p, resp.Application))
		},
	}

	return cmd
}
