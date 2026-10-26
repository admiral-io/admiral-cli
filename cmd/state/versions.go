package state

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

func newVersionsCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions
	cmd := &cobra.Command{
		Use:   "versions <app/env> <component>",
		Short: "List a component's state versions, newest first",
		Example: `  admiral state versions shop/prod network

  # The newest version's ID, for a download
  admiral state versions shop/prod network -o name | head -1`,
		Args:              flags.ExactArgs(2),
		ValidArgsFunction: complete.Positions(complete.Envs(opts), complete.ComponentsOf(opts, false)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			org, ws, err := workspace(cmd.Context(), c, cmd, args[0], args[1])
			if err != nil {
				return err
			}
			sc, err := client.NewStateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			versions, next, err := flags.Pages(paging, func(token string) ([]*client.StateVersion, string, error) {
				return sc.ListStateVersions(cmd.Context(), org, ws, paging.PageSize, token)
			})
			if err != nil {
				return err
			}
			items := make([]proto.Message, len(versions))
			for i, v := range versions {
				if items[i], err = asStruct(v); err != nil {
					return err
				}
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "state versions",
				Scope:         args[0] + " " + args[1],
				Items:         items,
				Name:          func(i int) string { return versions[i].ID },
				NextPageToken: next,
			}, versionTable.Render(p, versions...))
		},
	}
	flags.Paging(cmd, &paging)
	return cmd
}
