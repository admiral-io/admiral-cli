package catalog

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a catalog item",
		Long: `Print the one-line summary of a catalog item. Use 'describe' for the full view and '-o json' for the raw record.

The catalog item is given by name or ID.`,
		Example: `  # Get catalog item by name
  admiral catalog get billing-vpc

  # Get catalog item by UUID
  admiral catalog get 550e8400-e29b-41d4-a716-446655440000`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.CatalogItem(cmd.Context(), c.Catalog(), args[0])
			if err != nil {
				return err
			}

			resp, err := c.Catalog().GetCatalogItem(cmd.Context(), &catalogv1.GetCatalogItemRequest{
				CatalogItemId: id,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.CatalogItem, resp.CatalogItem.Name, catalogItemTable.Render(p, resp.CatalogItem))
		},
	}

	return cmd
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
