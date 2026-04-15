package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a credential",
		Long: `Create a credential by selecting an auth mechanism subcommand.

Subcommands:
  ssh-key        SSH private key (with optional passphrase)
  basic-auth     HTTP Basic auth (username + password)
  bearer-token   Single token value`,
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newCreateBasicAuthCmd(opts),
		newCreateBearerTokenCmd(opts),
		newCreateSSHKeyCmd(opts),
	)

	return cmd
}