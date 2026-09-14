package credential

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newCreateSSHKeyCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs       []string
		description     string
		privateKeyFile  string
		passphraseStdin bool
	)

	cmd := &cobra.Command{
		Use:   "ssh-key <name>",
		Short: "Create an SSH_KEY credential",
		Long:  `Create a credential holding an SSH private key (with optional passphrase), used for GIT sources accessed over SSH.`,
		Example: `  # From a key file (recommended)
  admiral credential create ssh-key acme-deploy \
    --private-key-file ~/.ssh/admiral_id_ed25519

  # With a passphrase (prompted on a terminal, or piped)
  admiral credential create ssh-key acme-deploy \
    --private-key-file ~/.ssh/key --passphrase-stdin < passphrase.txt`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			if privateKeyFile == "" {
				return cmderr.Usage("--private-key-file is required")
			}
			key, err := loadPrivateKey(privateKeyFile)
			if err != nil {
				return err
			}
			var passphrase string
			if passphraseStdin {
				if passphrase, err = input.Secret(cmd, "passphrase", true); err != nil {
					return err
				}
			}

			req := &credentialv1.CreateCredentialRequest{
				Name:        args[0],
				Description: description,
				Type:        credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY,
				Labels:      labels,
				AuthConfig: &credentialv1.AuthConfig{
					Variant: &credentialv1.AuthConfig_SshKey{
						SshKey: &credentialv1.SSHKeyAuth{PrivateKey: key, Passphrase: passphrase},
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
	cmd.Flags().StringVar(&privateKeyFile, "private-key-file", "", "path to SSH private key file")
	cmd.Flags().BoolVar(&passphraseStdin, "passphrase-stdin", false, "read the key passphrase from stdin (prompts on a terminal)")

	return cmd
}

// sshKeyRotation holds the flags used to rotate an SSH_KEY credential via
// `credential update`. Supports passphrase-only rotation.
type sshKeyRotation struct {
	privateKeyFile  string
	passphraseStdin bool
}

func (r *sshKeyRotation) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.privateKeyFile, "private-key-file", "", "(ssh-key) path to the new private key file")
	cmd.Flags().BoolVar(&r.passphraseStdin, "passphrase-stdin", false, "(ssh-key) read the new passphrase from stdin (prompts on a terminal)")
}

func (r *sshKeyRotation) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("private-key-file") || cmd.Flags().Changed("passphrase-stdin")
}

func (r *sshKeyRotation) apply(cmd *cobra.Command, cred *credentialv1.Credential) error {
	var key string
	if r.privateKeyFile != "" {
		k, err := loadPrivateKey(r.privateKeyFile)
		if err != nil {
			return err
		}
		key = k
	} else if sk := cred.GetAuthConfig().GetSshKey(); sk != nil {
		// Passphrase-only rotation: keep the existing key material.
		key = sk.PrivateKey
	}
	if key == "" {
		return cmderr.Usage("--private-key-file is required")
	}
	var passphrase string
	if r.passphraseStdin {
		p, err := input.Secret(cmd, "passphrase", true)
		if err != nil {
			return err
		}
		passphrase = p
	}
	cred.AuthConfig = &credentialv1.AuthConfig{
		Variant: &credentialv1.AuthConfig_SshKey{
			SshKey: &credentialv1.SSHKeyAuth{PrivateKey: key, Passphrase: passphrase},
		},
	}
	return nil
}

// loadPrivateKey returns the key material from either an inline --private-key
// value or a --private-key-file path. Flags are mutually exclusive. Returns
// "" when neither is supplied.
func loadPrivateKey(path string) (string, error) {
	expanded, err := input.ExpandPath(path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(expanded)
	if err != nil {
		return "", fmt.Errorf("read private key file: %w", err)
	}
	return string(b), nil
}
