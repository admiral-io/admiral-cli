package project

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		paging    flags.PagingOptions
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Long:  `List all projects visible to the current user.`,
		Example: `  # List all projects
  admiral project list

  # List with label filter
  admiral project list --label team=platform

  # Every project, however many pages the server splits them into
  admiral project list --all -o name

  # One page at a time
  admiral project list --page-size 10`,
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

			apps, next, err := flags.Pages(paging, func(token string) ([]*projectv1.Project, string, error) {
				resp, err := c.Project().ListProjects(cmd.Context(), &projectv1.ListProjectsRequest{
					PageSize:  paging.PageSize,
					PageToken: token,
					Filter:    filter,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Projects, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "projects",
				Items:         output.Messages(apps),
				Name:          func(i int) string { return apps[i].Name },
				NextPageToken: next,
			}, projectTable.Render(p, apps...))
		},
	}

	flags.Paging(cmd, &paging)
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")

	return cmd
}
