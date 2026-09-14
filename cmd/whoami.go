package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	userv1 "go.admiral.io/sdk/proto/admiral/api/user/v1"
)

func newWhoamiCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the identity the server sees",
		Long: `Ask the server who the active credential belongs to and show that user.
Use this to confirm a login worked. 'admiral auth status' shows the same
credential without contacting the server.`,
		Example: `  # Confirm the active credential against the server
  admiral whoami`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cred, err := credentials.ResolveToken(opts.ConfigDir)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.User().GetMe(cmd.Context(), &userv1.GetMeRequest{})
			if err != nil {
				return fmt.Errorf("failed to get user info: %w", err)
			}

			user := resp.GetUser()

			d := output.NewDescribe()
			d.Field("Email", user.GetEmail())
			d.Field("Display Name", user.GetDisplayName())
			d.Field("ID", user.GetId())
			d.Field("Server", opts.ServerAddr)
			d.Field("Auth", authLabel(cred, opts.ConfigDir))

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintStatus(resp.GetUser(), d)
		},
	}
}

// authLabel describes the credential in use, e.g. "api-key (ADMIRAL_API_KEY)".
func authLabel(cred *credentials.TokenResult, configDir string) string {
	switch cred.Source {
	case credentials.SourceEnv:
		return "api-key (" + credentials.EnvAPIKey + ")"
	case credentials.SourceAPIKey:
		if c, err := credentials.Load(configDir); err == nil && c.Kind == credentials.KindAPIKeyRef {
			return "api-key (" + c.Ref + ")"
		}
		return "api-key (auth login --with-token)"
	case credentials.SourceSession:
		return "session (auth login)"
	}
	return string(cred.Source)
}
