package app

import (
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
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
  admctl app list

  # List with label filter
  admctl app list --label team=platform

  # Paginated listing
  admctl app list --page-size 10`,
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
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.Application().ListApplications(cmd.Context(), &applicationv1.ListApplicationsRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			if err := p.PrintResource(resp, func(w *tabwriter.Writer) {
				if opts.OutputFormat == output.FormatWide {
					output.Writeln(w, "ID\tNAME\tDESCRIPTION\tLABELS\tCREATED BY\tUPDATED BY\tCREATED\tUPDATED\tAGE")
					for _, app := range resp.Applications {
						output.Writef(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
							app.Id,
							app.Name,
							app.Description,
							output.FormatLabels(app.Labels),
							app.CreatedBy,
							app.UpdatedBy,
							output.FormatTimestamp(app.CreatedAt),
							output.FormatTimestamp(app.UpdatedAt),
							output.FormatAge(app.CreatedAt),
						)
					}
				} else {
					output.Writeln(w, "NAME\tDESCRIPTION\tAGE")
					for _, app := range resp.Applications {
						output.Writef(w, "%s\t%s\t%s\n",
							app.Name,
							app.Description,
							output.FormatAge(app.CreatedAt),
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
