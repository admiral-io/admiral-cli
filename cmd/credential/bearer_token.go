package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newCreateBearerTokenCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs   []string
		description string
		tokenStdin  bool
	)

	cmd := &cobra.Command{
		Use:   "bearer-token <name>",
		Short: "Create a BEARER_TOKEN credential",
		Long:  `Create a credential holding a single token value, reusable across TERRAFORM, HELM, OCI, and HTTP sources.`,
		Example: `  # Create with flags
  admiral credential create bearer-token hcp-terraform \
    --token xxxxx.atlasv1.xxxxxxxxxxxxxx

  # Read token from stdin (recommended)
  echo "$TF_TOKEN" | admiral credential create bearer-token hcp-terraform --token-stdin

  # Prompt for token interactively
  admiral credential create bearer-token hcp-terraform`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			tk, err := input.Secret(cmd, "token", tokenStdin)
			if err != nil {
				return err
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_BEARER_TOKEN,
				Labels:      labels,
				AuthConfig: &credentialv1.AuthConfig{
					Variant: &credentialv1.AuthConfig_BearerToken{
						BearerToken: &credentialv1.BearerTokenAuth{Token: tk},
					},
				},
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Credential().CreateCredential(cmd.Context(), req)
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	flags.Label(cmd, &labelStrs, "label to attach (key=value, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "credential description")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read token from stdin")

	return cmd
}

// bearerTokenRotation holds the flags used to rotate a BEARER_TOKEN credential
// via `credential update`.
type bearerTokenRotation struct {
	tokenStdin bool
}

func (r *bearerTokenRotation) bind(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&r.tokenStdin, "token-stdin", false, "(BEARER_TOKEN) read new token from stdin")
}

func (r *bearerTokenRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("token-stdin")
}

func (r *bearerTokenRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	tk, err := input.Secret(cmd, "token", r.tokenStdin)
	if err != nil {
		return err
	}
	cred.AuthConfig = &credentialv1.AuthConfig{
		Variant: &credentialv1.AuthConfig_BearerToken{
			BearerToken: &credentialv1.BearerTokenAuth{Token: tk},
		},
	}
	return nil
}
