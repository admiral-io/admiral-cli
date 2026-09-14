package catalog

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List catalog items",
		Long:  `List all catalog items visible to the current user.`,
		Example: `  # List all catalog items
  admiral catalog list

  # List with label filter
  admiral catalog list --label team=platform

  # Paginated listing
  admiral catalog list --page-size 10`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := flags.LabelFilter(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Catalog().ListCatalogItems(cmd.Context(), &catalogv1.ListCatalogItemsRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "catalog items",
				Items:         output.Messages(resp.CatalogItems),
				Name:          func(i int) string { return resp.CatalogItems[i].Name },
				NextPageToken: resp.NextPageToken,
			}, catalogItemTable.Render(p, resp.CatalogItems...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")
	return cmd
}
