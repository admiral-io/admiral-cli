package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents",
		Long:  `List all agents visible to the current user.`,
		Example: `  # List all agents
  admiral agent list

  # Filter by label
  admiral agent list --label team=platform

  # Paginated listing
  admiral agent list --page-size 10`,
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

			resp, err := c.Agent().ListAgents(cmd.Context(), &agentv1.ListAgentsRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "agents",
				Items:         output.Messages(resp.Agents),
				Name:          func(i int) string { return resp.Agents[i].Name },
				NextPageToken: resp.NextPageToken,
			}, agentTable.Render(p, resp.Agents...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")
	return cmd
}
