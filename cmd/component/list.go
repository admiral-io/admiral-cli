package component

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		paging    flags.PagingOptions
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List components in the registry",
		Example: `  # List every component
  admiral component list

  # Filter by label
  admiral component list --label team=payments

  # Names only, for scripting
  admiral component list -o name`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := flags.LabelFilter(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			comps, next, err := flags.Pages(paging, func(token string) ([]*registryv1.Component, string, error) {
				resp, err := c.Registry().ListComponents(cmd.Context(), &registryv1.ListComponentsRequest{
					Filter:    filter,
					PageSize:  paging.PageSize,
					PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Components, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "components",
				Items:         output.Messages(comps),
				Name:          func(i int) string { return comps[i].Name },
				NextPageToken: next,
			}, componentTable.Render(p, comps...))
		},
	}

	flags.Paging(cmd, &paging)
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")

	return cmd
}
