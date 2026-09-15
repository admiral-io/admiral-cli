package app

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List applications",
		Long:  `List all applications visible to the current user.`,
		Example: `  # List all applications
  admiral app list

  # List with label filter
  admiral app list --label team=platform

  # Paginated listing
  admiral app list --page-size 10`,
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
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.Application().ListApplications(cmd.Context(), &applicationv1.ListApplicationsRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "applications",
				Items:         output.Messages(resp.Applications),
				Name:          func(i int) string { return resp.Applications[i].Name },
				NextPageToken: resp.NextPageToken,
			}, appTable.Render(p, resp.Applications...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")

	return cmd
}
