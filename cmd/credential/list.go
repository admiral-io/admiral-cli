package credential

import (
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		pageSize  int32
		pageToken string
		labelStrs []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credentials",
		Long:  `List all credentials visible to the current user.`,
		Example: `  # List all credentials
  admiral credential list

  # List with label filter
  admiral credential list --label team=platform

  # Paginated listing
  admiral credential list --page-size 10`,
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

			resp, err := c.Credential().ListCredentials(cmd.Context(), &credentialv1.ListCredentialsRequest{
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
			})
			if err != nil {
				return err
			}

			if len(resp.Credentials) == 0 && (opts.OutputFormat == output.FormatTable || opts.OutputFormat == output.FormatWide) {
				output.PrintEmpty(cmd.ErrOrStderr(), "credentials")
				return nil
			}

			p := output.NewPrinter(opts.OutputFormat)
			if err := p.PrintResource(resp, func(w *tabwriter.Writer) {
				if opts.OutputFormat == output.FormatWide {
					output.Writeln(w, "ID\tNAME\tTYPE\tDESCRIPTION\tLABELS\tCREATED\tAGE")
					for _, c := range resp.Credentials {
						output.Writef(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
							c.Id, c.Name, formatCredentialType(c.Type), c.Description,
							output.FormatLabels(c.Labels),
							output.FormatTimestamp(c.CreatedAt),
							output.FormatAge(c.CreatedAt),
						)
					}
				} else {
					output.Writeln(w, "NAME\tTYPE\tDESCRIPTION\tAGE")
					for _, c := range resp.Credentials {
						output.Writef(w, "%s\t%s\t%s\t%s\n",
							c.Name, formatCredentialType(c.Type), c.Description,
							output.FormatAge(c.CreatedAt),
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
