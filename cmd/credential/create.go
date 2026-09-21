package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <type> <name>",
		Short: "Create a credential",
		Long: `Create a credential of one of these types:

  basic-auth     a user name and password: chart repositories, container
                 registries (a robot account, a PAT as the password)
  bearer-token   a token sent as a bearer on HTTPS, and as the password
                 with user x-access-token on OCI: a GitHub PAT
  github-app     an app id, installation id and private key, exchanged
                 for a short-lived installation token on every pull
  ssh-key        a private key for git over SSH

The secret is piped on stdin, read from a file, or prompted for with echo
off; it is never a flag. --allowed-host restricts where the credential may
be presented: with none set it is offered to any host its type fits.`,
		Example: `  # A GitHub PAT, prompted for, only ever sent to github.com and ghcr.io
  admiral credential create bearer-token github --allowed-host github.com --allowed-host ghcr.io

  # A registry robot account from a secret store
  op read op://infra/harbor-robot/password | admiral credential create basic-auth harbor \
    --username 'robot$pull' --password-stdin --allowed-host harbor.acme.example:8443

  # A deploy key
  admiral credential create ssh-key infra-deploy --private-key-file ~/.ssh/infra_deploy

  # A GitHub App installed on the org
  admiral credential create github-app acme-app --app-id 12345 --installation-id 67890 \
    --private-key-file ./acme-app.pem`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	for _, t := range typeNames {
		cmd.AddCommand(newCreateTypeCmd(opts, t))
	}
	return cmd
}

func newCreateTypeCmd(opts *client.Options, typ string) *cobra.Command {
	var (
		description  string
		labelStrs    []string
		allowedHosts []string
		secret       secretFlags
	)

	cmd := &cobra.Command{
		Use:   typ + " <name>",
		Short: "Create a " + typ + " credential",
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			auth, err := secret.authConfig(cmd, typ)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Credential().CreateCredential(cmd.Context(), &credentialv1.CreateCredentialRequest{
				Name:         args[0],
				Description:  description,
				Type:         typeEnum[typ],
				AuthConfig:   auth,
				Labels:       labels,
				AllowedHosts: allowedHosts,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	secret.register(cmd, typ)
	cmd.Flags().StringArrayVar(&allowedHosts, "allowed-host", nil, "host this credential may be presented to, with a port if not the default (repeatable; default: any)")
	cmd.Flags().StringVar(&description, "description", "", "credential description")
	flags.Label(cmd, &labelStrs, "label to attach (key=value, repeatable)")

	return cmd
}
