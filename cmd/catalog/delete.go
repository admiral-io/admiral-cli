package catalog

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a catalog item",
		Long: `Delete a catalog item.

The catalog item is given by name or ID.`,
		Example: `  # Delete a catalog item by name (prompts to confirm)
  admiral catalog delete billing-vpc

  # Skip the prompt
  admiral catalog delete billing-vpc --force`,
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

			display := args[0]
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Delete catalog item %s", display)); err != nil {
				return err
			}

			if _, err := c.Catalog().DeleteCatalogItem(cmd.Context(), &catalogv1.DeleteCatalogItemRequest{
				CatalogItemId: id,
			}); err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "catalog item", display, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
