package credential

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

func newCreateBearerTokenCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs   []string
		description string
		token       string
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
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := util.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			tk, err := input.ResolveSecret(cmd, "token", token, tokenStdin)
			if err != nil {
				return err
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_BEARER_TOKEN,
				Labels:      labels,
				AuthConfig: &credentialv1.CreateCredentialRequest_BearerToken{
					BearerToken: &credentialv1.BearerTokenAuth{Token: tk},
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
			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				printCredentialRow(w, resp.Credential)
			})
		},
	}

	util.AddLabelFlag(cmd, &labelStrs, "label to attach (key=value, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "credential description")
	cmd.Flags().StringVar(&token, "token", "", "bearer token value (avoid in shell history; prefer --token-stdin or interactive prompt)")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read token from stdin")

	return cmd
}

// bearerTokenRotation holds the flags used to rotate a BEARER_TOKEN credential
// via `credential update`.
type bearerTokenRotation struct {
	token      string
	tokenStdin bool
}

func (r *bearerTokenRotation) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.token, "token", "", "(BEARER_TOKEN) new token (avoid in shell history)")
	cmd.Flags().BoolVar(&r.tokenStdin, "token-stdin", false, "(BEARER_TOKEN) read new token from stdin")
}

func (r *bearerTokenRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("token") || cmd.Flags().Changed("token-stdin")
}

func (r *bearerTokenRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	tk, err := input.FromFlagOrStdin("token", r.token, r.tokenStdin)
	if err != nil {
		return err
	}
	if tk == "" {
		return fmt.Errorf("--token or --token-stdin is required")
	}
	cred.AuthConfig = &credentialv1.Credential_BearerToken{
		BearerToken: &credentialv1.BearerTokenAuth{Token: tk},
	}
	return nil
}
