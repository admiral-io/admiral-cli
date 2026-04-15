package credential

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

// authRotation is implemented by each per-type rotation struct
// (basicAuthRotation, bearerTokenRotation, sshKeyRotation). bind registers
// its flags; changed reports whether any were supplied; apply rewrites
// cred.AuthConfig in place when the user wants to rotate.
type authRotation interface {
	bind(cmd *cobra.Command)
	changed(cmd *cobra.Command) bool
	apply(cmd *cobra.Command, cred *credentialv1.Credential) error
}

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		credID      string
		newName     string
		description string
		labelStrs   []string

		basic  basicAuthRotation
		bearer bearerTokenRotation
		ssh    sshKeyRotation
	)

	cmd := &cobra.Command{
		Use:   "update [credential]",
		Short: "Update a credential",
		Long: `Update an existing credential.

Updatable fields: name, description, labels, and the secret material in
auth_config (password, token, private key). The credential's TYPE is
immutable -- to switch type, delete and recreate.

When rotating secrets, only flags valid for the credential's type are
accepted. For example, --token applies to BEARER_TOKEN credentials only.`,
		Example: `  # Rename
  admiral credential update acme-github --name acme-github-pat

  # Rotate a BASIC_AUTH password (read from stdin)
  echo "$NEW_PAT" | admiral credential update acme-github --password-stdin

  # Rotate a BEARER_TOKEN
  admiral credential update hcp-terraform --token xxxxx.atlasv1.NEW

  # Update labels
  admiral credential update acme-github --label team=platform`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && credID == "" {
				_ = cmd.Help()
				return fmt.Errorf("credential name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveCredentialID(cmd.Context(), c.Credential(), nameArg, credID)
			if err != nil {
				return err
			}

			// Read-modify-write so untouched fields keep their values.
			current, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{
				CredentialId: id,
			})
			if err != nil {
				return err
			}
			cred := current.Credential

			var paths []string
			if cmd.Flags().Changed("name") {
				cred.Name = newName
				paths = append(paths, "name")
			}
			if cmd.Flags().Changed("description") {
				cred.Description = description
				paths = append(paths, "description")
			}
			if cmd.Flags().Changed("label") {
				labels, err := util.ParseLabels(labelStrs)
				if err != nil {
					return err
				}
				cred.Labels = labels
				paths = append(paths, "labels")
			}

			authChanged, err := applyRotation(cmd, cred, &basic, &bearer, &ssh)
			if err != nil {
				return err
			}
			if authChanged {
				paths = append(paths, "auth_config")
			}

			if len(paths) == 0 {
				return fmt.Errorf("at least one of --name, --description, --label, or auth-rotation flags must be specified")
			}

			resp, err := c.Credential().UpdateCredential(cmd.Context(), &credentialv1.UpdateCredentialRequest{
				Credential: cred,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				printCredentialRow(w, resp.Credential)
			})
		},
	}

	cmd.Flags().StringVar(&credID, "id", "", "credential ID (UUID)")
	cmd.Flags().StringVar(&newName, "name", "", "new credential name")
	cmd.Flags().StringVar(&description, "description", "", "credential description")
	util.AddLabelFlag(cmd, &labelStrs, "label to set (key=value, repeatable)")

	basic.bind(cmd)
	bearer.bind(cmd)
	ssh.bind(cmd)

	return cmd
}

// applyRotation dispatches to the rotation matching cred.Type. Returns true
// when auth_config was rewritten so the caller can add it to the update mask.
// Errors if rotation flags from a different type were supplied.
func applyRotation(cmd *cobra.Command, cred *credentialv1.Credential, basic *basicAuthRotation, bearer *bearerTokenRotation, ssh *sshKeyRotation) (bool, error) {
	usingBasic := basic.changed(cmd)
	usingBearer := bearer.changed(cmd)
	usingSSH := ssh.changed(cmd)

	switch {
	case !usingBasic && !usingBearer && !usingSSH:
		return false, nil
	case (usingBasic && usingBearer) || (usingBasic && usingSSH) || (usingBearer && usingSSH):
		return false, fmt.Errorf("auth rotation flags from different credential types are mutually exclusive")
	}

	switch cred.Type {
	case credentialv1.CredentialType_CREDENTIAL_TYPE_BASIC_AUTH:
		if !usingBasic {
			return false, fmt.Errorf("credential type is BASIC_AUTH; use --username / --password / --password-stdin")
		}
		return true, basic.apply(cmd, cred)
	case credentialv1.CredentialType_CREDENTIAL_TYPE_BEARER_TOKEN:
		if !usingBearer {
			return false, fmt.Errorf("credential type is BEARER_TOKEN; use --token / --token-stdin")
		}
		return true, bearer.apply(cmd, cred)
	case credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY:
		if !usingSSH {
			return false, fmt.Errorf("credential type is SSH_KEY; use --private-key / --private-key-file / --passphrase")
		}
		return true, ssh.apply(cmd, cred)
	default:
		return false, fmt.Errorf("unsupported credential type: %s", cred.Type)
	}
}