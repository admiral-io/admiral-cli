package apikey

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type APIKeyCmd struct {
	Cmd *cobra.Command
}

// NewAPIKeyCmd manages the caller's own API keys. The secret is shown once,
// when the key is created; list shows the record.
func NewAPIKeyCmd(opts *client.Options) *APIKeyCmd {
	root := &APIKeyCmd{}
	cmd := &cobra.Command{
		Use:   "apikey",
		Short: "Manage your API keys",
		Long: `Manage your own API keys.

An API key signs you in where a browser cannot: CI, a script, or Terraform
reaching hosted state. It acts as you, limited to the scopes it was created
with; what you may do is still decided by your permissions.

The secret is printed once, on create, and cannot be shown again.

Commands accept a key name or ID as the positional argument.`,
		Aliases:       []string{"apikeys"},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)
	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newRevokeCmd(opts),
	)
	root.Cmd = cmd
	return root
}
