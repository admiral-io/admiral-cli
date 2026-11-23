package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a source",
		Long:  `Get a source. A credential it holds is named, never shown.`,
		Example: `  admiral source get argo-helm

  admiral source get argo-helm -o yaml`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Sources(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(src, src.Name, sourceTable.Render(p, src))
		},
	}
	return cmd
}
