package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type CredentialCmd struct {
	Cmd *cobra.Command
}

func NewCredentialCmd(opts *client.Options) *CredentialCmd {
	root := &CredentialCmd{}

	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage credentials",
		Long: `Manage credentials used to authenticate to external systems.

Credentials are mechanism-rooted: the type describes the auth shape
(SSH key, basic auth, bearer token), not the target system. A single
credential may be reused across many Sources of compatible types.`,
		Aliases:       []string{"cred", "credentials"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}
