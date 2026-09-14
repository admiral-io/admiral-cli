package catalog

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <name>",
		Short:   "Show everything about a catalog item",
		Long:    "Render a catalog item's identity and where its content comes from.\n\ndescribe is a human view. Use 'catalog get -o json' for the raw record.",
		Example: "  admiral catalog describe vpc",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
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
			resp, err := c.Catalog().GetCatalogItem(cmd.Context(), &catalogv1.GetCatalogItemRequest{CatalogItemId: id})
			if err != nil {
				return err
			}
			m := resp.CatalogItem

			d := output.NewDescribe()
			d.Field("Name", m.Name)
			d.Field("ID", m.Id)
			d.Field("Type", output.FormatEnumKebab(m.Type))
			d.Field("Description", m.Description)
			d.Fields("Labels", output.LabelLines(m.Labels))
			d.Field("Created", output.FormatDescribeTime(m.CreatedAt))
			d.Field("Created By", output.FormatActor(m.CreatedBy))
			d.Field("Updated", output.FormatDescribeTime(m.UpdatedAt))
			d.Section("Source", func(b *output.Block) {
				b.Field("Name", orID(m.SourceName, m.SourceId))
				b.Field("Ref", refOrDefault(m.Ref))
				b.Field("Root", m.Root)
				b.Field("Path", m.Path)
			})
			d.Hint("To resolve the ref to a commit, run: admiral catalog resolve " + m.Name)

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral catalog get "+m.Name)
		},
	}
	return cmd
}
