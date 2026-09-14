package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a source",
		Long: `Print the one-line summary of a source. Use 'describe' for the full view and '-o json' for the raw record.

The source is given by name or ID.`,
		Example: `  # Get source by name
  admiral source get acme-infra

  # Get source by UUID
  admiral source get 550e8400-e29b-41d4-a716-446655440000`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}

			resp, err := c.Source().GetSource(cmd.Context(), &sourcev1.GetSourceRequest{
				SourceId: id,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Source, resp.Source.Name, sourceTable.Render(p, resp.Source))
		},
	}

	return cmd
}
