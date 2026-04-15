package credential

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	var credID string

	cmd := &cobra.Command{
		Use:   "get [credential]",
		Short: "Get credential details",
		Long: `Get detailed information about a credential.

The credential can be provided as a positional argument (name) or looked up by UUID with --id.
Sensitive fields (tokens, keys, passwords) are never returned.`,
		Example: `  # Get credential by name
  admiral credential get acme-github

  # Get credential by UUID
  admiral credential get --id 550e8400-e29b-41d4-a716-446655440000`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && credID == "" {
				_ = cmd.Help()
				return fmt.Errorf("credential name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveCredentialID(cmd.Context(), c.Credential(), nameArg, credID)
			if err != nil {
				return err
			}

			resp, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{
				CredentialId: id,
			})
			if err != nil {
				return err
			}
			cred := resp.Credential

			p := output.NewPrinter(opts.OutputFormat)
			sections := []output.Section{{
				Details: []output.Detail{
					{Key: "ID", Value: cred.Id},
					{Key: "Name", Value: cred.Name},
					{Key: "Type", Value: formatCredentialType(cred.Type)},
					{Key: "Description", Value: cred.Description},
					{Key: "Labels", Value: output.FormatLabels(cred.Labels)},
					{Key: "Created", Value: output.FormatTimestamp(cred.CreatedAt)},
					{Key: "Updated", Value: output.FormatTimestamp(cred.UpdatedAt)},
					{Key: "Age", Value: output.FormatAge(cred.CreatedAt)},
				},
			}}
			return p.PrintDetail(resp, sections)
		},
	}

	cmd.Flags().StringVar(&credID, "id", "", "credential ID (UUID)")
	return cmd
}
