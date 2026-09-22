package credential

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newRotateCmd(opts *client.Options) *cobra.Command {
	var secret secretFlags

	cmd := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Replace a credential's secret",
		Long: `Replace a credential's secret with a new one of the same type. The record,
its labels and its allowed hosts stay; pulls after this present the new
material. Which flags apply depends on the credential's type, as for
'credential create'; a GitHub App gives its ids again, since nothing of the
old configuration is read back.`,
		Example: `  # A new PAT, prompted for
  admiral credential rotate github

  # A new robot password from a secret store
  op read op://infra/harbor-robot/password | admiral credential rotate harbor --username 'robot$pull' --password-stdin

  # A new deploy key
  admiral credential rotate infra-deploy --private-key-file ~/.ssh/infra_deploy_2

  # A new app key
  admiral credential rotate acme-app --app-id 12345 --installation-id 67890 --private-key-file ./acme-app-2.pem`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Credentials(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Credential(cmd.Context(), c.Credential(), args[0])
			if err != nil {
				return err
			}
			current, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{CredentialId: id})
			if err != nil {
				return err
			}
			cred := current.Credential
			auth, err := secret.authConfig(cmd, typeName(cred.Type))
			if err != nil {
				return err
			}
			cred.AuthConfig = auth

			resp, err := c.Credential().UpdateCredential(cmd.Context(), &credentialv1.UpdateCredentialRequest{
				Credential: cred,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"auth_config"}},
			})
			if err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "credential", cred.Name, "rotated")
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	secret.register(cmd, typeNames...)

	return cmd
}
