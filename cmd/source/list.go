package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sources",
		Example: `  admiral source list

  # Names only, for scripting
  admiral source list -o name`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			sources, next, err := flags.Pages(paging, func(token string) ([]*sourcev1.Source, string, error) {
				resp, err := c.Source().ListSources(cmd.Context(), &sourcev1.ListSourcesRequest{
					PageSize: paging.PageSize, PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Sources, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "sources",
				Items:         output.Messages(sources),
				Name:          func(i int) string { return sources[i].Name },
				NextPageToken: next,
			}, sourceTable.Render(p, sources...))
		},
	}

	flags.Paging(cmd, &paging)
	return cmd
}
