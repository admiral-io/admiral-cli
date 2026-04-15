package source

import (
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
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
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := util.BuildLabelFilter(labelStrs)
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

			if len(resp.Sources) == 0 && (opts.OutputFormat == output.FormatTable || opts.OutputFormat == output.FormatWide) {
				output.PrintEmpty(cmd.ErrOrStderr(), "sources")
				return nil
			}

			p := output.NewPrinter(opts.OutputFormat)
			if err := p.PrintResource(resp, func(w *tabwriter.Writer) {
				if opts.OutputFormat == output.FormatWide {
					output.Writeln(w, "ID\tNAME\tTYPE\tURL\tCREDENTIAL ID\tCATALOG\tLAST TEST\tCREATED\tAGE")
					for _, s := range resp.Sources {
						credID := ""
						if s.CredentialId != nil {
							credID = *s.CredentialId
						}
						output.Writef(w, "%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\n",
							s.Id, s.Name, formatSourceType(s.Type), s.Url, credID, s.Catalog,
							formatTestStatus(s.LastTestStatus),
							output.FormatTimestamp(s.CreatedAt),
							output.FormatAge(s.CreatedAt),
						)
					}
				} else {
					output.Writeln(w, "NAME\tTYPE\tURL\tLAST TEST\tAGE")
					for _, s := range resp.Sources {
						output.Writef(w, "%s\t%s\t%s\t%s\t%s\n",
							s.Name, formatSourceType(s.Type), s.Url,
							formatTestStatus(s.LastTestStatus),
							output.FormatAge(s.CreatedAt),
						)
					}
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

	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	util.AddLabelFlag(cmd, &labelStrs, "filter by label (key=value, repeatable)")
	return cmd
}