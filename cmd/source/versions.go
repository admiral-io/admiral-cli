package source

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newVersionsCmd(opts *client.Options) *cobra.Command {
	var (
		srcID     string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "versions [source]",
		Short: "List available versions for a source",
		Long:  `Query the external system for available versions (git tags, registry releases, etc.).`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && srcID == "" {
				_ = cmd.Help()
				return fmt.Errorf("source name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveSourceID(cmd.Context(), c.Source(), nameArg, srcID)
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

			if len(resp.Versions) == 0 && (opts.OutputFormat == output.FormatTable || opts.OutputFormat == output.FormatWide) {
				output.PrintEmpty(cmd.ErrOrStderr(), "versions")
				return nil
			}

			p := output.NewPrinter(opts.OutputFormat)
			if err := p.PrintResource(resp, func(w *tabwriter.Writer) {
				output.Writeln(w, "VERSION\tPUBLISHED\tDESCRIPTION")
				for _, v := range resp.Versions {
					output.Writef(w, "%s\t%s\t%s\n",
						v.Version,
						output.FormatTimestamp(v.PublishedAt),
						v.Description,
					)
				}
			}); err != nil {
				return err
			}
			if resp.NextPageToken != "" && opts.OutputFormat != output.FormatJSON && opts.OutputFormat != output.FormatYAML {
				output.Writef(cmd.ErrOrStderr(), "\nNEXT PAGE TOKEN: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	return cmd
}
