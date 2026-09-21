package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type CredentialCmd struct {
	Cmd *cobra.Command
}

// NewCredentialCmd manages the credentials a pull may present. The secret
// is written once and never read back; what is listed is the record.
func NewCredentialCmd(opts *client.Options) *CredentialCmd {
	root := &CredentialCmd{}

	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage credentials for pulling private artifacts",
		Long: `Manage credentials for pulling private artifacts.

A credential is a secret Admiral holds for the tenant and presents on its
behalf: an SSH key for a git host, a username and password for a chart or
container registry, a bearer token for an HTTPS host, or a GitHub App that
is exchanged for an installation token per pull. It is never resolved by
itself: 'component pull --credential NAME' attaches it, and it is presented
only where its type fits the protocol and its allowed hosts permit.

The secret is written on create and on update, never returned: get and
list show the record, not the material.

Commands accept a credential name or ID as the positional argument.`,
		Aliases:       []string{"credentials", "cred"},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newUpdateCmd(opts),
		newRotateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}
