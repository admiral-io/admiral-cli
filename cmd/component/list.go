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
		pageSize  int32
		pageToken string
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

			resp, err := c.Registry().ListComponents(cmd.Context(), &registryv1.ListComponentsRequest{
				Filter:    filter,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "components",
				Items:         output.Messages(resp.Components),
				Name:          func(i int) string { return resp.Components[i].Name },
				NextPageToken: resp.NextPageToken,
			}, componentTable.Render(p, resp.Components...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")

	return cmd
}
