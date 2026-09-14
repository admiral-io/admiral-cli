package credential

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a credential",
		Long: `Delete a credential.

The credential is given by name or ID.`,
		Example: `  # Delete a credential by name (prompts to confirm)
  admiral credential delete acme-github

  # Skip the prompt
  admiral credential delete acme-github --force`,
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

			display := args[0]
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Delete credential %s", display)); err != nil {
				return err
			}

			if _, err := c.Credential().DeleteCredential(cmd.Context(), &credentialv1.DeleteCredentialRequest{
				CredentialId: id,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "credential", display, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
