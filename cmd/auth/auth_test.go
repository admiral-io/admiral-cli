package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
)

// validKey passes the SDK's opaque-format check (prefix, length, checksum).
const validKey = "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG"

// stubOp puts a fake `op` on PATH that prints out, or fails with stderr when
// fail is set, and counts invocations in a file.
func stubOp(t *testing.T, out string, fail bool) func() int {
	t.Helper()
	bin := t.TempDir()
	counter := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho x >> " + counter + "\n"
	if fail {
		script += "echo 'vault is locked' >&2; exit 1\n"
	} else {
		script += "printf '%s' '" + out + "'\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(bin, "op"), []byte(script), 0755)) //nolint:gosec // test stub
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		b, _ := os.ReadFile(counter)
		return strings.Count(string(b), "x")
	}
}

func run(t *testing.T, opts *client.Options, stdin string, args ...string) (stdout string, err error) {
	t.Helper()
	root := NewAuthCmd(opts).Cmd
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

func TestLoginWithAPIKey_StoresKey(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	stdout, err := run(t, opts, validKey+"\n", "login", "--with-token")
	require.NoError(t, err)
	require.Contains(t, stdout, "API key stored")

	cred, err := credentials.Load(opts.ConfigDir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindAPIKey, cred.Kind)
	require.Equal(t, validKey, cred.APIKey)

	// It is now what commands will use, with the Token scheme.
	res, err := credentials.ResolveToken(opts.ConfigDir)
	require.NoError(t, err)
	require.Equal(t, credentials.SourceAPIKey, res.Source)
}

func TestLoginWithAPIKey_RejectsMalformed(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	_, err := run(t, opts, "not-a-key\n", "login", "--with-token")
	require.ErrorContains(t, err, "invalid API key")

	_, err = credentials.Load(opts.ConfigDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoginWithAPIKey_EmptyInput(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	_, err := run(t, opts, "", "login", "--with-token")
	require.Error(t, err)
}

func TestLoginWithAPIKey_ReplacesSession(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindSession, AccessToken: "jwt",
	}))

	_, err := run(t, opts, validKey+"\n", "login", "--with-token")
	require.NoError(t, err)

	cred, err := credentials.Load(opts.ConfigDir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindAPIKey, cred.Kind)
	require.Empty(t, cred.AccessToken)
}

func TestLogin_RefusesWhenEnvKeySet(t *testing.T) {
	t.Setenv(credentials.EnvAPIKey, validKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	for _, args := range [][]string{{"login"}, {"login", "--with-token"}} {
		_, err := run(t, opts, validKey+"\n", args...)
		require.ErrorContains(t, err, credentials.EnvAPIKey)
		require.ErrorContains(t, err, "first clear it from the environment")
	}

	_, err := credentials.Load(opts.ConfigDir)
	require.ErrorIs(t, err, os.ErrNotExist, "nothing stored")
}

func TestLogin_FlagsMutuallyExclusive(t *testing.T) {
	opts := &client.Options{ConfigDir: t.TempDir()}
	_, err := run(t, opts, "", "login", "--with-token", "--no-browser")
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestLogout_StoredAPIKey(t *testing.T) {
	opts := &client.Options{ConfigDir: t.TempDir()}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindAPIKey, APIKey: validKey,
	}))

	stdout, err := run(t, opts, "", "logout")
	require.NoError(t, err)
	require.Contains(t, stdout, "API key removed")

	_, err = credentials.Load(opts.ConfigDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLogout_NothingStored(t *testing.T) {
	opts := &client.Options{ConfigDir: t.TempDir()}
	stdout, err := run(t, opts, "", "logout")
	require.NoError(t, err)
	require.Contains(t, stdout, "Not logged in")
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name   string
		env    string
		stored *credentials.Credential
		want   status
	}{
		{"nothing", "", nil, status{Authenticated: false}},
		{"env", validKey, nil, status{Authenticated: true, Method: "api-key", Storage: "environment"}},
		{"stored key", "", &credentials.Credential{Kind: credentials.KindAPIKey, APIKey: validKey},
			status{Authenticated: true, Method: "api-key", Storage: "file", Path: "FILE"}},
		{"session", "", &credentials.Credential{Kind: credentials.KindSession, AccessToken: "jwt", Issuer: "https://idp", Email: "m@x"},
			status{Authenticated: true, Method: "session", Storage: "file", Path: "FILE", Issuer: "https://idp", Account: "m@x"}},
		{"env beats stored", validKey, &credentials.Credential{Kind: credentials.KindSession, AccessToken: "jwt"},
			status{Authenticated: true, Method: "api-key", Storage: "environment"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv(credentials.EnvAPIKey, tc.env)
			} else {
				os.Unsetenv(credentials.EnvAPIKey)
			}
			opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
			if tc.stored != nil {
				require.NoError(t, credentials.Save(opts.ConfigDir, tc.stored))
			}

			stdout, err := run(t, opts, "", "status")
			require.NoError(t, err)

			var got status
			require.NoError(t, json.Unmarshal([]byte(stdout), &got))
			got.Error = "" // wording is asserted elsewhere
			if tc.want.Path == "FILE" {
				tc.want.Path = credentials.FilePath(opts.ConfigDir)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoginWithToken_StoresReference(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}
	calls := stubOp(t, validKey, false)

	stdout, err := run(t, opts, "op://Vault/admiral/key\n", "login", "--with-token")
	require.NoError(t, err)
	require.Contains(t, stdout, "reference stored: op://Vault/admiral/key")
	require.Equal(t, 1, calls(), "reference is resolved once at store time")

	cred, err := credentials.Load(opts.ConfigDir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindAPIKeyRef, cred.Kind)
	require.Equal(t, "op://Vault/admiral/key", cred.Ref)
	require.Empty(t, cred.APIKey)
}

func TestLoginWithToken_ReferenceResolvesToBadKey(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}
	stubOp(t, "not-a-key", false)

	_, err := run(t, opts, "op://Vault/admiral/key\n", "login", "--with-token")
	require.ErrorContains(t, err, "invalid API key")

	_, err = credentials.Load(opts.ConfigDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoginWithToken_ReferenceUnresolvable(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}
	stubOp(t, "", true)

	_, err := run(t, opts, "op://Vault/admiral/key\n", "login", "--with-token")
	require.ErrorContains(t, err, "vault is locked")
}

func TestStatus_ShowsReference(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
	stubOp(t, validKey, false)
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindAPIKeyRef, Ref: "op://Vault/admiral/key",
	}))

	stdout, err := run(t, opts, "", "status")
	require.NoError(t, err)
	var got status
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.True(t, got.Authenticated)
	require.Equal(t, "api-key", got.Method)
	require.Equal(t, "1password", got.Storage)
	require.Equal(t, "op://Vault/admiral/key", got.Ref)
	require.Empty(t, got.Path)
}

func TestLogout_StoredReference(t *testing.T) {
	opts := &client.Options{ConfigDir: t.TempDir()}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindAPIKeyRef, Ref: "op://Vault/admiral/key",
	}))

	stdout, err := run(t, opts, "", "logout")
	require.NoError(t, err)
	require.Contains(t, stdout, "reference removed")
}

func TestValidateScopes(t *testing.T) {
	got, err := validateScopes([]string{"app:read,env:read", "app:read", "run:read"})
	require.NoError(t, err)
	require.Equal(t, []string{"app:read", "env:read", "run:read"}, got, "comma-split, deduplicated, order kept")

	_, err = validateScopes([]string{"app:*"})
	require.Error(t, err, "scopes attenuate; there are no family wildcards")

	got, err = validateScopes(nil)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = validateScopes([]string{"app-read"})
	require.ErrorContains(t, err, `unknown scope "app-read"`)
	require.ErrorContains(t, err, "app:read")

	_, err = validateScopes([]string{"openid"})
	require.Error(t, err, "identity scopes are not user-selectable")

	_, err = validateScopes([]string{"agent:exec"})
	require.Error(t, err, "agent-only scope is not assignable to a person")
}

func TestUserScopes(t *testing.T) {
	all := userScopes()
	require.Contains(t, all, "app:read")
	require.NotContains(t, all, "app:*", "no wildcards; scopes are concrete attenuations")
	require.NotContains(t, all, "agent:exec", "agent-only scope is not assignable to a person")
	require.IsIncreasing(t, all)
}

func TestLogin_ScopeFlagValidation(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	_, err := run(t, opts, "", "login", "--scope", "bogus:read")
	require.ErrorContains(t, err, `unknown scope "bogus:read"`)

	_, err = run(t, opts, validKey+"\n", "login", "--with-token", "--scope", "app:read")
	require.ErrorContains(t, err, "--scope applies to browser sign-in")
}

func TestStatus_ShowsScopes(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindSession, AccessToken: "jwt", Scopes: []string{"app:read"},
	}))

	stdout, err := run(t, opts, "", "status")
	require.NoError(t, err)
	var got status
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []string{"app:read"}, got.Scopes)
}
