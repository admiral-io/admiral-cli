package credential

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
)

// run executes the command with an empty, non-terminal stdin, so every
// prompt is an error naming its flag.
func run(t *testing.T, args ...string) error {
	t.Helper()
	root := NewCredentialCmd(&client.Options{})
	root.Cmd.SetIn(strings.NewReader(""))
	root.Cmd.SetOut(io.Discard)
	root.Cmd.SetErr(io.Discard)
	root.Cmd.SetArgs(args)
	return root.Cmd.Execute()
}

func TestPositionalUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get"}, "missing argument: get <name>"},
		{[]string{"list", "extra"}, `unknown argument "extra"`},
		{[]string{"create", "bearer-token"}, "missing argument: bearer-token <name>"},
		{[]string{"update"}, "missing argument: update <name>"},
		{[]string{"rotate"}, "missing argument: rotate <name>"},
		{[]string{"delete"}, "missing argument: delete <name>"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := run(t, tc.args...)
			require.EqualError(t, err, tc.want)
			assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		})
	}
}

// The secret is checked before any network: a run that cannot prompt and
// did not pipe fails in milliseconds, naming the flag.
func TestSecretsAreRefusedLocallyWhatCanBe(t *testing.T) {
	pem := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(pem, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n"), 0o600))
	notPEM := filepath.Join(t.TempDir(), "id_rsa")
	require.NoError(t, os.WriteFile(notPEM, []byte("ssh-rsa AAAA"), 0o600))

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"create", "basic-auth", "harbor"}, "--username is required"},
		{[]string{"create", "basic-auth", "harbor", "--username", "robot"}, "--password-stdin required when not running interactively"},
		{[]string{"create", "bearer-token", "gh"}, "--token-stdin required when not running interactively"},
		{[]string{"create", "ssh-key", "deploy"}, "--private-key-file or --private-key-stdin is required"},
		{[]string{"create", "ssh-key", "deploy", "--private-key-file", notPEM}, "not PEM"},
		{[]string{"create", "ssh-key", "deploy", "--private-key-file", pem, "--private-key-stdin"}, "not both"},
		{[]string{"create", "ssh-key", "deploy", "--private-key-stdin", "--passphrase-stdin"}, "cannot both read stdin"},
		{[]string{"create", "github-app", "app", "--private-key-file", pem}, "--app-id and --installation-id are required"},
		{[]string{"create", "github-app", "app", "--app-id", "1", "--installation-id", "2"}, "--private-key-file or --private-key-stdin is required"},
		{[]string{"update", "gh"}, "at least one of"},
		{[]string{"update", "gh", "--allowed-host", "a", "--clear-allowed-hosts"}, "not both"},
		{[]string{"delete", "gh"}, "--force required when not running interactively"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := run(t, tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		})
	}
}

func TestPrivateKeyIsReadWhole(t *testing.T) {
	key := "-----BEGIN RSA PRIVATE KEY-----\nline1\nline2\n-----END RSA PRIVATE KEY-----\r\n"
	got, err := checkPEM(key)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimRight(key, "\r\n")+"\n", got, "one trailing newline, the body intact")
	_, err = checkPEM("")
	require.Error(t, err)
}

func TestFormatHosts(t *testing.T) {
	assert.Equal(t, "any", formatHosts(nil))
	assert.Equal(t, "github.com,ghcr.io", formatHosts([]string{"github.com", "ghcr.io"}))
}
