package component

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a component and its tags",
		Example: `  admiral component get cloud-sql

  # The full object, tags with their digests
  admiral component get cloud-sql -o yaml`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Registry().GetComponent(cmd.Context(), &registryv1.GetComponentRequest{Name: args[0]})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Component, resp.Component.Name, componentTable.Render(p, resp.Component))
		},
	}

	return cmd
}
