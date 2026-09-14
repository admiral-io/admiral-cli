package catalog

import (
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		newName    string
		desc       string
		sourceName string
		ref        string
		root       string
		path       string
		labelStrs  []string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a catalog item",
		Long: `Update an existing catalog item.

Updateable: name, description, source, ref, root, path, labels.
The catalog item's TYPE is immutable.`,
		Example: `  # Change the ref
  admiral catalog update billing-vpc --ref v2.0.0

  # Repoint to a different source
  admiral catalog update billing-vpc --source new-infra-repo

  # Update root and path
  admiral catalog update billing-vpc --root modules/vpc-v2 --path environments/prod`,
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

			current, err := c.Catalog().GetCatalogItem(cmd.Context(), &catalogv1.GetCatalogItemRequest{CatalogItemId: id})
			if err != nil {
				return err
			}
			m := current.CatalogItem

			var paths []string
			if cmd.Flags().Changed("name") {
				m.Name = newName
				paths = append(paths, "name")
			}
			if cmd.Flags().Changed("description") {
				m.Description = desc
				paths = append(paths, "description")
			}
			if cmd.Flags().Changed("ref") {
				m.Ref = ref
				paths = append(paths, "ref")
			}
			if cmd.Flags().Changed("root") {
				m.Root = root
				paths = append(paths, "root")
			}
			if cmd.Flags().Changed("path") {
				m.Path = path
				paths = append(paths, "path")
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(m.Labels, labelStrs)
				if err != nil {
					return err
				}
				m.Labels = labels
				paths = append(paths, "labels")
			}

			srcChanged := cmd.Flags().Changed("source")
			if srcChanged {
				resolved, err := resolve.Source(cmd.Context(), c.Source(), sourceName)
				if err != nil {
					return err
				}
				m.SourceId = resolved
				paths = append(paths, "source_id")
			}

			if len(paths) == 0 {
				return fmt.Errorf("at least one updateable field must be specified")
			}

			resp, err := c.Catalog().UpdateCatalogItem(cmd.Context(), &catalogv1.UpdateCatalogItemRequest{
				CatalogItem: m,
				UpdateMask:  &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.CatalogItem, resp.CatalogItem.Name, catalogItemTable.Render(p, resp.CatalogItem))
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new catalog item name")
	cmd.Flags().StringVar(&desc, "description", "", "catalog item description")
	cmd.Flags().StringVar(&sourceName, "source", "", "source name or ID to repoint to")
	cmd.Flags().StringVar(&ref, "ref", "", "git ref / version")
	cmd.Flags().StringVar(&root, "root", "", "subdirectory within fetched tree")
	cmd.Flags().StringVar(&path, "path", "", "working directory within root")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	return cmd
}
