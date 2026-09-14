package catalog

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newResolveCmd(opts *client.Options) *cobra.Command {
	var (
		refOverride string
	)

	cmd := &cobra.Command{
		Use:   "resolve <name>",
		Short: "Resolve a catalog item",
		Long: `Fetch the catalog item via its source and return the resolved revision and digest.

Does not stream content. Use --ref to override the catalog item's default ref
without modifying the catalog item definition.`,
		Example: `  # Resolve using the catalog item's default ref
  admiral catalog resolve billing-vpc

  # Resolve at a specific ref
  admiral catalog resolve billing-vpc --ref v2.0.0`,
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

			resp, err := c.Catalog().ResolveCatalogItem(cmd.Context(), &catalogv1.ResolveCatalogItemRequest{
				CatalogItemId: id,
				RefOverride:   refOverride,
			})
			if err != nil {
				return err
			}

			d := output.NewDescribe()
			d.Field("Catalog Item", resp.CatalogItem.Name)
			d.Field("Type", output.FormatEnumKebab(resp.CatalogItem.Type))
			d.Field("Source", orID(resp.CatalogItem.SourceName, resp.CatalogItem.SourceId))
			d.Field("Ref", refOrDefault(resp.CatalogItem.Ref))
			d.Field("Revision", resp.Revision)
			d.Field("Digest", resp.Digest)

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintStatus(resp, d)
		},
	}

	cmd.Flags().StringVar(&refOverride, "ref", "", "override the catalog item's default ref for this resolve")
	return cmd
}
