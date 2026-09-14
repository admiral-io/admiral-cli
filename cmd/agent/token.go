package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

func newTokenCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage agent service access tokens",
		Long: `Manage an agent's service access tokens (SATs).

Rotation flow: 'token create' to issue a new token, roll it into the
agent's deployment, verify the agent is healthy, then 'token revoke'
the old one.`,
		Aliases: []string{"tokens"},
		Args:    flags.NoArgs,
	}

	cmd.AddCommand(
		newTokenCreateCmd(opts),
		newTokenListCmd(opts),
		newTokenGetCmd(opts),
		newTokenRevokeCmd(opts),
	)
	return cmd
}
