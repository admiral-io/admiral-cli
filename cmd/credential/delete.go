package credential

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var credID string

	cmd := &cobra.Command{
		Use:   "delete [credential]",
		Short: "Delete a credential",
		Long: `Delete a credential.

The credential can be provided as a positional argument (name) or looked up by UUID with --id.`,
		Example: `  # Delete a credential by name
  admiral credential delete acme-github

  # Delete by UUID
  admiral credential delete --id 550e8400-e29b-41d4-a716-446655440000`,
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

			if _, err := c.Credential().DeleteCredential(cmd.Context(), &credentialv1.DeleteCredentialRequest{
				CredentialId: id,
			}); err != nil {
				return err
			}

			output.Writef(cmd.OutOrStdout(), "Credential %s deleted\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&credID, "id", "", "credential ID (UUID)")
	return cmd
}