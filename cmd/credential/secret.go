package credential

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/iostreams"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

// The type names the CLI uses, the kebab form of the enum.
const (
	typeSSHKey      = "ssh-key"
	typeBasicAuth   = "basic-auth"
	typeBearerToken = "bearer-token"
	typeGitHubApp   = "github-app"
)

var typeEnum = map[string]credentialv1.CredentialType{
	typeSSHKey:      credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY,
	typeBasicAuth:   credentialv1.CredentialType_CREDENTIAL_TYPE_BASIC_AUTH,
	typeBearerToken: credentialv1.CredentialType_CREDENTIAL_TYPE_BEARER_TOKEN,
	typeGitHubApp:   credentialv1.CredentialType_CREDENTIAL_TYPE_GITHUB_APP,
}

var typeNames = []string{typeBasicAuth, typeBearerToken, typeGitHubApp, typeSSHKey}

func typeName(t credentialv1.CredentialType) string {
	for name, e := range typeEnum {
		if e == t {
			return name
		}
	}
	return t.String()
}

// secretFlags are the inputs that carry the secret. A secret never travels
// in a flag: it is piped on stdin, read from a file, or prompted for with
// echo off (style guide §1.3). Each type registers the subset it needs;
// rotate registers all of them and checks against the record's type.
type secretFlags struct {
	username        string
	passwordStdin   bool
	tokenStdin      bool
	privateKeyFile  string
	privateKeyStdin bool
	passphraseStdin bool
	appID           int64
	installationID  int64
	apiURL          string
}

func (f *secretFlags) register(cmd *cobra.Command, types ...string) {
	for _, t := range types {
		switch t {
		case typeBasicAuth:
			cmd.Flags().StringVar(&f.username, "username", "", "the user name")
			cmd.Flags().BoolVar(&f.passwordStdin, "password-stdin", false, "read the password from stdin (default: prompt)")
		case typeBearerToken:
			cmd.Flags().BoolVar(&f.tokenStdin, "token-stdin", false, "read the token from stdin (default: prompt)")
		case typeSSHKey:
			f.registerPrivateKey(cmd, "the PEM-encoded private key")
			cmd.Flags().BoolVar(&f.passphraseStdin, "passphrase-stdin", false, "read the key's passphrase from stdin (default: none)")
		case typeGitHubApp:
			cmd.Flags().Int64Var(&f.appID, "app-id", 0, "the app's numeric id, from its settings page")
			cmd.Flags().Int64Var(&f.installationID, "installation-id", 0, "the installation's numeric id, from its URL")
			cmd.Flags().StringVar(&f.apiURL, "api-url", "", "the REST API base for GitHub Enterprise Server (default: https://api.github.com)")
			f.registerPrivateKey(cmd, "the app's PEM-encoded private key")
		}
	}
}

// registerPrivateKey is shared by ssh-key and github-app; rotate registers
// both types, so a second call must not redefine the flags.
func (f *secretFlags) registerPrivateKey(cmd *cobra.Command, what string) {
	if cmd.Flags().Lookup("private-key-file") != nil {
		return
	}
	cmd.Flags().StringVar(&f.privateKeyFile, "private-key-file", "", "read "+what+" from this file")
	cmd.Flags().BoolVar(&f.privateKeyStdin, "private-key-stdin", false, "read "+what+" from stdin")
}

// authConfig reads the secret for typ the way the flags say and builds
// the write-only message.
func (f *secretFlags) authConfig(cmd *cobra.Command, typ string) (*credentialv1.AuthConfig, error) {
	switch typ {
	case typeBasicAuth:
		if f.username == "" {
			return nil, cmderr.Usage("--username is required")
		}
		password, err := input.Secret(cmd, "password", f.passwordStdin)
		if err != nil {
			return nil, err
		}
		return &credentialv1.AuthConfig{Variant: &credentialv1.AuthConfig_BasicAuth{
			BasicAuth: &credentialv1.BasicAuth{Username: f.username, Password: password},
		}}, nil
	case typeBearerToken:
		token, err := input.Secret(cmd, "token", f.tokenStdin)
		if err != nil {
			return nil, err
		}
		return &credentialv1.AuthConfig{Variant: &credentialv1.AuthConfig_BearerToken{
			BearerToken: &credentialv1.BearerTokenAuth{Token: token},
		}}, nil
	case typeSSHKey:
		if f.privateKeyStdin && f.passphraseStdin {
			return nil, cmderr.Usage("--private-key-stdin and --passphrase-stdin cannot both read stdin; pass the key with --private-key-file")
		}
		key, err := f.privateKey(cmd)
		if err != nil {
			return nil, err
		}
		var passphrase string
		if f.passphraseStdin {
			if passphrase, err = input.Secret(cmd, "passphrase", true); err != nil {
				return nil, err
			}
		}
		return &credentialv1.AuthConfig{Variant: &credentialv1.AuthConfig_SshKey{
			SshKey: &credentialv1.SSHKeyAuth{PrivateKey: key, Passphrase: passphrase},
		}}, nil
	case typeGitHubApp:
		if f.appID <= 0 || f.installationID <= 0 {
			return nil, cmderr.Usage("--app-id and --installation-id are required")
		}
		key, err := f.privateKey(cmd)
		if err != nil {
			return nil, err
		}
		return &credentialv1.AuthConfig{Variant: &credentialv1.AuthConfig_GithubApp{
			GithubApp: &credentialv1.GitHubAppAuth{
				AppId: f.appID, InstallationId: f.installationID, PrivateKey: key, ApiUrl: f.apiURL,
			},
		}}, nil
	}
	return nil, fmt.Errorf("unknown credential type %q", typ)
}

// privateKey reads PEM material from the file or the pipe. There is no
// prompt: a key is many lines, and typing one is not a thing.
func (f *secretFlags) privateKey(cmd *cobra.Command) (string, error) {
	switch {
	case f.privateKeyFile != "" && f.privateKeyStdin:
		return "", cmderr.Usage("pass either --private-key-file or --private-key-stdin, not both")
	case f.privateKeyFile != "":
		path, err := input.ExpandPath(f.privateKeyFile)
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read private key: %w", err)
		}
		return checkPEM(string(b))
	case f.privateKeyStdin:
		ios := iostreams.FromCommand(cmd)
		if ios.IsStdinTTY() {
			return "", cmderr.Usage("--private-key-stdin reads a pipe; a key is not typed")
		}
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", fmt.Errorf("read private key from stdin: %w", err)
		}
		return checkPEM(string(b))
	}
	return "", cmderr.Usage("--private-key-file or --private-key-stdin is required")
}

// checkPEM refuses what is plainly not a key before it is sealed and stored:
// a path typed where the file was meant, an empty pipe.
func checkPEM(s string) (string, error) {
	s = strings.TrimRight(s, "\r\n")
	if !strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN ") {
		return "", cmderr.Usage("the private key is not PEM (expected -----BEGIN ...)")
	}
	return s + "\n", nil
}
