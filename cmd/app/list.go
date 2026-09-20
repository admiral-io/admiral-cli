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
		paging    flags.PagingOptions
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

  # Every application, however many pages the server splits them into
  admiral app list --all -o name

  # One page at a time
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

			apps, next, err := flags.Pages(paging, func(token string) ([]*applicationv1.Application, string, error) {
				resp, err := c.Application().ListApplications(cmd.Context(), &applicationv1.ListApplicationsRequest{
					PageSize:  paging.PageSize,
					PageToken: token,
					Filter:    filter,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Applications, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "applications",
				Items:         output.Messages(apps),
				Name:          func(i int) string { return apps[i].Name },
				NextPageToken: next,
			}, appTable.Render(p, apps...))
		},
	}

	flags.Paging(cmd, &paging)
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")

	return cmd
}
