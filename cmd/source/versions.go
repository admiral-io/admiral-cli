package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newVersionsCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "versions <name>",
		Short: "List available versions for a source",
		Long: `Query the external system for available refs: git branches and tags,
Terraform/Helm registry versions, or OCI tags. Each row shows its KIND and,
where available, the immutable handle it RESOLVED to (a git commit).`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Source().ListSourceVersions(cmd.Context(), &sourcev1.ListSourceVersionsRequest{
				SourceId:  id,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "versions",
				Items:         output.Messages(resp.Versions),
				Name:          func(i int) string { return resp.Versions[i].Version },
				NextPageToken: resp.NextPageToken,
			}, versionTable.Render(p, resp.Versions...))
		},
	}

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	return cmd
}
