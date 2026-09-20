package auth

import (
	"github.com/spf13/cobra"

	internalauth "go.admiral.io/cli/internal/auth"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

func newLogoutCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and discard the stored credential",
		Long: `Remove the credential stored by 'admiral auth login'. For a browser session
the refresh token is also revoked at the identity provider. ADMIRAL_API_KEY,
if set, is not affected.`,
		Example: `  # Log out and remove the stored credential
  admiral auth logout`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := internalauth.Logout(cmd.Context(), opts.ConfigDir)
			if err != nil {
				return err
			}
			switch {
			case !res.Removed:
				output.Writeln(cmd.OutOrStdout(), "Not logged in.")
			case res.Kind == "":
				// The file was there but unreadable, so what it held is
				// unknown. It is gone; say that rather than "not logged in".
				output.Writeln(cmd.OutOrStdout(), "Removed an unreadable credentials file. Run 'admiral auth login' to sign in again.")
			case res.Kind == credentials.KindAPIKey:
				output.Writeln(cmd.OutOrStdout(), "Stored API key removed.")
			case res.Kind == credentials.KindAPIKeyRef:
				output.Writeln(cmd.OutOrStdout(), "Stored API key reference removed. The key itself is untouched in its store.")
			default:
				output.Writeln(cmd.OutOrStdout(), "Logged out.")
			}
			return nil
		},
	}
}
