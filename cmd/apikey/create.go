package apikey

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/scopes"
	apikeyv1 "go.admiral.io/sdk/proto/admiral/api/apikey/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var scopeFlag []string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an API key and print its secret",
		Long: `Create an API key limited to the given scopes, and print its secret on
stdout. The secret is shown this once.`,
		Example: `  # A key for Terraform to reach hosted state
  admiral apikey create laptop-terraform --scope state:write
  export TF_TOKEN_state_admiral_io=$(admiral apikey create ci-state --scope state:read)

  # A CI key that reads applications and environments
  admiral apikey create ci-read --scope app:read,env:read`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := scopes.Validate(scopeFlag)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				return cmderr.Usage("--scope is required: a key carries only the scopes it is created with")
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			resp, err := c.ApiKey().CreateApiKey(cmd.Context(), &apikeyv1.CreateApiKeyRequest{
				ApiKey: &apikeyv1.ApiKey{Name: args[0], Scopes: list},
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.GetSecret())
			fmt.Fprintf(cmd.ErrOrStderr(), "API key %s created. Store the secret now; Admiral cannot show it again.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&scopeFlag, "scope", nil, "scope the key carries (repeatable or comma-separated)")
	_ = cmd.RegisterFlagCompletionFunc("scope", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return scopes.User(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}
