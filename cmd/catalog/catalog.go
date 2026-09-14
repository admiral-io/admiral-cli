package catalog

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type CatalogCmd struct {
	Cmd *cobra.Command
}

func NewCatalogCmd(opts *client.Options) *CatalogCmd {
	root := &CatalogCmd{}

	cmd := &cobra.Command{
		Use:           "catalog",
		Short:         "Manage catalog items",
		Long:          `Manage catalog items -- named, reusable references to content within a Source at a specific ref and path.`,
		Aliases:       []string{"cat", "catalog-items"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
		newResolveCmd(opts),
	)

	root.Cmd = cmd
	return root
}
