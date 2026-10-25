package apikey

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	apikeyv1 "go.admiral.io/sdk/proto/admiral/api/apikey/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your API keys",
		Example: `  admiral apikey list
  # Names only, for scripting
  admiral apikey list -o name`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			keys, next, err := flags.Pages(paging, func(token string) ([]*apikeyv1.ApiKey, string, error) {
				resp, err := c.ApiKey().ListApiKeys(cmd.Context(), &apikeyv1.ListApiKeysRequest{
					PageSize: paging.PageSize, PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.ApiKeys, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "api keys",
				Items:         output.Messages(keys),
				Name:          func(i int) string { return keys[i].Name },
				NextPageToken: next,
			}, apiKeyTable.Render(p, keys...))
		},
	}
	flags.Paging(cmd, &paging)
	return cmd
}
