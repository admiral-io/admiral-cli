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

func newCreateBasicAuthCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs     []string
		description   string
		username      string
		password      string
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
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := util.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			if username == "" {
				return fmt.Errorf("--username is required")
			}
			pw, err := input.ResolveSecret(cmd, "password", password, passwordStdin)
			if err != nil {
				return err
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_BASIC_AUTH,
				Labels:      labels,
				AuthConfig: &credentialv1.CreateCredentialRequest_BasicAuth{
					BasicAuth: &credentialv1.BasicAuth{Username: username, Password: pw},
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
	cmd.Flags().StringVar(&username, "username", "", "basic auth username (required)")
	cmd.Flags().StringVar(&password, "password", "", "basic auth password (avoid in shell history; prefer --password-stdin or interactive prompt)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin")

	return cmd
}

// basicAuthRotation holds the flags used to rotate a BASIC_AUTH credential's
// secret material via `credential update`.
type basicAuthRotation struct {
	username      string
	password      string
	passwordStdin bool
}

func (r *basicAuthRotation) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.username, "username", "", "(BASIC_AUTH) new username")
	cmd.Flags().StringVar(&r.password, "password", "", "(BASIC_AUTH) new password (avoid in shell history)")
	cmd.Flags().BoolVar(&r.passwordStdin, "password-stdin", false, "(BASIC_AUTH) read new password from stdin")
}

func (r *basicAuthRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("username") || cmd.Flags().Changed("password") || cmd.Flags().Changed("password-stdin")
}

func (r *basicAuthRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	if !cmd.Flags().Changed("password") && !cmd.Flags().Changed("password-stdin") {
		return fmt.Errorf("--password or --password-stdin is required to rotate BASIC_AUTH")
	}
	pw, err := input.FromFlagOrStdin("password", r.password, r.passwordStdin)
	if err != nil {
		return err
	}

	// Preserve existing username when only the password is being rotated.
	newUser := ""
	if ba := cred.GetBasicAuth(); ba != nil {
		newUser = ba.Username
	}
	if cmd.Flags().Changed("username") {
		newUser = r.username
	}
	if newUser == "" {
		return fmt.Errorf("--username is required (current credential has no username on record)")
	}

	cred.AuthConfig = &credentialv1.Credential_BasicAuth{
		BasicAuth: &credentialv1.BasicAuth{Username: newUser, Password: pw},
	}
	return nil
}