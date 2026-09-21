package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a credential",
		Long: `Delete a credential. The secret is destroyed with the record; what was
pulled with it stays published.`,
		Example: `  admiral credential delete github

  admiral credential delete github --force`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Credentials(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.RequireInteractiveOrForce(cmd, force); err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Credential(cmd.Context(), c.Credential(), args[0])
			if err != nil {
				return err
			}
			if err := input.Confirm(cmd, force, "Delete credential "+args[0]); err != nil {
				return err
			}
			if _, err := c.Credential().DeleteCredential(cmd.Context(), &credentialv1.DeleteCredentialRequest{CredentialId: id}); err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "credential", args[0], "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
