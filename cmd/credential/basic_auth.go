package credential

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newCreateBasicAuthCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs     []string
		description   string
		username      string
		passwordStdin bool
	)

	cmd := &cobra.Command{
		Use:   "basic-auth <name>",
		Short: "Create a BASIC_AUTH credential",
		Long:  `Create a credential holding HTTP Basic credentials (username + password), reusable across GIT, HELM, OCI, and HTTP sources.`,
		Example: `  # Create with flags
  admiral credential create basic-auth acme-github \
    --username your-username --password ghp_xxxxxxxxxxxx

  # Read password from stdin (recommended)
  echo "ghp_xxxxxxxxxxxx" | admiral credential create basic-auth acme-github \
    --username your-username --password-stdin

  # Prompt for password interactively
  admiral credential create basic-auth acme-github --username your-username`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			if username == "" {
				return cmderr.Usage("--username is required")
			}
			pw, err := input.Secret(cmd, "password", passwordStdin)
			if err != nil {
				return err
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_BASIC_AUTH,
				Labels:      labels,
				AuthConfig: &credentialv1.AuthConfig{
					Variant: &credentialv1.AuthConfig_BasicAuth{
						BasicAuth: &credentialv1.BasicAuth{Username: username, Password: pw},
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
	cmd.Flags().StringVar(&username, "username", "", "basic auth username (required)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin")

	return cmd
}

// basicAuthRotation holds the flags used to rotate a BASIC_AUTH credential's
// secret material via `credential update`.
type basicAuthRotation struct {
	username      string
	passwordStdin bool
}

func (r *basicAuthRotation) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.username, "username", "", "(BASIC_AUTH) new username")
	cmd.Flags().BoolVar(&r.passwordStdin, "password-stdin", false, "(BASIC_AUTH) read new password from stdin")
}

func (r *basicAuthRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("username") || cmd.Flags().Changed("password-stdin")
}

func (r *basicAuthRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	pw, err := input.Secret(cmd, "password", r.passwordStdin)
	if err != nil {
		return err
	}

	// Preserve existing username when only the password is being rotated.
	newUser := ""
	if ba := cred.GetAuthConfig().GetBasicAuth(); ba != nil {
		newUser = ba.Username
	}
	if cmd.Flags().Changed("username") {
		newUser = r.username
	}
	if newUser == "" {
		return fmt.Errorf("--username is required (current credential has no username on record)")
	}

	cred.AuthConfig = &credentialv1.AuthConfig{
		Variant: &credentialv1.AuthConfig_BasicAuth{
			BasicAuth: &credentialv1.BasicAuth{Username: newUser, Password: pw},
		},
	}
	return nil
}
