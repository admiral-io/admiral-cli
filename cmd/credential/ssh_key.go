package credential

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
)

func newCreateSSHKeyCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs      []string
		description    string
		privateKey     string
		privateKeyFile string
		passphrase     string
	)

	cmd := &cobra.Command{
		Use:   "ssh-key <name>",
		Short: "Create an SSH_KEY credential",
		Long:  `Create a credential holding an SSH private key (with optional passphrase), used for GIT sources accessed over SSH.`,
		Example: `  # From a key file (recommended)
  admiral credential create ssh-key acme-deploy \
    --private-key-file ~/.ssh/admiral_id_ed25519

  # With passphrase
  admiral credential create ssh-key acme-deploy \
    --private-key-file ~/.ssh/key --passphrase 's3cret'`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := util.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			key, err := loadPrivateKey(privateKey, privateKeyFile)
			if err != nil {
				return err
			}
			if key == "" {
				return fmt.Errorf("--private-key or --private-key-file is required")
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY,
				Labels:      labels,
				AuthConfig: &credentialv1.CreateCredentialRequest_SshKey{
					SshKey: &credentialv1.SSHKeyAuth{PrivateKey: key, Passphrase: passphrase},
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
	cmd.Flags().StringVar(&privateKey, "private-key", "", "PEM-encoded SSH private key (avoid in shell; prefer --private-key-file)")
	cmd.Flags().StringVar(&privateKeyFile, "private-key-file", "", "path to SSH private key file")
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "optional passphrase for an encrypted key")

	return cmd
}

// sshKeyRotation holds the flags used to rotate an SSH_KEY credential via
// `credential update`. Supports passphrase-only rotation.
type sshKeyRotation struct {
	privateKey     string
	privateKeyFile string
	passphrase     string
}

func (r *sshKeyRotation) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.privateKey, "private-key", "", "(SSH_KEY) new PEM private key (avoid in shell history)")
	cmd.Flags().StringVar(&r.privateKeyFile, "private-key-file", "", "(SSH_KEY) path to new private key file")
	cmd.Flags().StringVar(&r.passphrase, "passphrase", "", "(SSH_KEY) passphrase for the new key")
}

func (r *sshKeyRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("private-key") || cmd.Flags().Changed("private-key-file") || cmd.Flags().Changed("passphrase")
}

func (r *sshKeyRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	key, err := loadPrivateKey(r.privateKey, r.privateKeyFile)
	if err != nil {
		return err
	}
	if key == "" && cmd.Flags().Changed("passphrase") {
		// Passphrase-only rotation: keep the existing key material.
		if sk := cred.GetSshKey(); sk != nil {
			key = sk.PrivateKey
		}
	}
	if key == "" {
		return fmt.Errorf("--private-key or --private-key-file is required (or passphrase-only rotation needs an existing key)")
	}
	cred.AuthConfig = &credentialv1.Credential_SshKey{
		SshKey: &credentialv1.SSHKeyAuth{PrivateKey: key, Passphrase: r.passphrase},
	}
	return nil
}

// loadPrivateKey returns the key material from either an inline --private-key
// value or a --private-key-file path. Flags are mutually exclusive. Returns
// "" when neither is supplied.
func loadPrivateKey(inline, path string) (string, error) {
	if inline != "" && path != "" {
		return "", fmt.Errorf("--private-key and --private-key-file are mutually exclusive")
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read private key file: %w", err)
		}
		return string(b), nil
	}
	return inline, nil
}