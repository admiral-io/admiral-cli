package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a credential",
		Long: `Print the one-line summary of a credential. Use 'describe' for the full view and '-o json' for the raw record.

The credential is given by name or ID.
Sensitive fields (tokens, keys, passwords) are never returned.`,
		Example: `  # Get credential by name
  admiral credential get acme-github

  # Get credential by UUID
  admiral credential get 550e8400-e29b-41d4-a716-446655440000`,
		Args: flags.ExactArgs(1),
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

			resp, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{
				CredentialId: id,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	return cmd
}
