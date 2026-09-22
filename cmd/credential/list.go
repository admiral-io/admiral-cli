package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		paging    flags.PagingOptions
		labelStrs []string
		typ       string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credentials",
		Example: `  admiral credential list

  # Only the SSH keys
  admiral credential list --type ssh-key

  # Names only, for scripting
  admiral credential list -o name`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := flags.LabelFilter(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			creds, next, err := flags.Pages(paging, func(token string) ([]*credentialv1.Credential, string, error) {
				resp, err := c.Credential().ListCredentials(cmd.Context(), &credentialv1.ListCredentialsRequest{
					Filter:    filter,
					Type:      typeEnum[typ],
					PageSize:  paging.PageSize,
					PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Credentials, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "credentials",
				Items:         output.Messages(creds),
				Name:          func(i int) string { return creds[i].Name },
				NextPageToken: next,
			}, credentialTable.Render(p, creds...))
		},
	}

	flags.Paging(cmd, &paging)
	flags.Label(cmd, &labelStrs, "filter by label (key=value, repeatable)")
	flags.Enum(cmd, &typ, "type", "", "only credentials of this type", typeNames...)

	return cmd
}
