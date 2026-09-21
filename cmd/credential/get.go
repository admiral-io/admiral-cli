package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a credential",
		Long:  `Get a credential's record. The secret is never returned.`,
		Example: `  admiral credential get github

  admiral credential get github -o yaml`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Credentials(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Credential(cmd.Context(), c.Credential(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{CredentialId: id})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	return cmd
}
