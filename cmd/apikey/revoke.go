package apikey

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	apikeyv1 "go.admiral.io/sdk/proto/admiral/api/apikey/v1"
)

func newRevokeCmd(opts *client.Options) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <name>",
		Short: "Revoke an API key",
		Long: `Revoke an API key. Anything signed in with it stops working within
minutes, once its current access token expires.`,
		Example: `  admiral apikey revoke laptop-terraform
  admiral apikey revoke laptop-terraform --yes`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			id, err := resolve.PersonalAccessToken(cmd.Context(), c.ApiKey(), args[0])
			if err != nil {
				return err
			}
			if err := input.Confirm(cmd, yes, "Revoke API key "+args[0]); err != nil {
				return err
			}
			if _, err := c.ApiKey().RevokeApiKey(cmd.Context(), &apikeyv1.RevokeApiKeyRequest{ApiKeyId: id}); err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "API key", args[0], "revoked")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
