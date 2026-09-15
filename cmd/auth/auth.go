// Package auth provides the `admiral auth` command group: interactive login,
// logout, and inspection of the active credential.
package auth

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type AuthCmd struct {
	Cmd *cobra.Command
}

func NewAuthCmd(opts *client.Options) *AuthCmd {
	root := &AuthCmd{}

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long: `Manage how the CLI authenticates to Admiral.

  admiral auth login               Browser sign-in; stores a session that
                                   refreshes itself.
  admiral auth login --with-token  Store an API key instead (no browser).
  ADMIRAL_API_KEY=<key>            Supply an API key per invocation, for CI.

The stored credential lives in credentials.json inside the config directory.
If ADMIRAL_API_KEY is set, it takes precedence over the stored credential.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newLoginCmd(opts),
		newLogoutCmd(opts),
		newStatusCmd(opts),
	)

	root.Cmd = cmd
	return root
}
