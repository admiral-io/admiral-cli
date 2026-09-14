package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sources",
		Long:  `List all sources visible to the current user.`,
		Example: `  # List all sources
  admiral source list

  # List with label filter
  admiral source list --label team=platform

  # Paginated listing
  admiral source list --page-size 10`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := flags.LabelFilter(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Source().ListSources(cmd.Context(), &sourcev1.ListSourcesRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "sources",
				Items:         output.Messages(resp.Sources),
				Name:          func(i int) string { return resp.Sources[i].Name },
				NextPageToken: resp.NextPageToken,
			}, sourceTable.Render(p, resp.Sources...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")
	return cmd
}
