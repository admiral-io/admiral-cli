package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	userv1 "go.admiral.io/sdk/proto/admiral/api/user/v1"
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
	res, err := credentials.ResolveToken(context.Background(), opts.ConfigDir)
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

// An empty pipe is a wiring mistake in the calling script: a usage error
// naming stdin, not a complaint about the key's format.
func TestLoginWithAPIKey_EmptyInput(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir()}

	_, err := run(t, opts, "", "login", "--with-token")
	require.EqualError(t, err, "no API key on stdin")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
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
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
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

// A file that cannot be parsed is removed, and the message says so: "Not
// logged in" would claim there had been nothing to remove.
func TestLogout_UnreadableFile(t *testing.T) {
	opts := &client.Options{ConfigDir: t.TempDir()}
	path := filepath.Join(opts.ConfigDir, "credentials.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0600))

	stdout, err := run(t, opts, "", "logout")
	require.NoError(t, err)
	require.Contains(t, stdout, "unreadable credentials file")
	require.NotContains(t, stdout, "Not logged in")
	require.NoFileExists(t, path)
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name   string
		env    string
		stored *credentials.Credential
		want   authStatus
	}{
		{"nothing", "", nil, authStatus{Authenticated: false}},
		{"env", validKey, nil, authStatus{Authenticated: true, Method: "api-key", Storage: "environment"}},
		{"stored key", "", &credentials.Credential{Kind: credentials.KindAPIKey, APIKey: validKey},
			authStatus{Authenticated: true, Method: "api-key", Storage: "file", Path: "FILE"}},
		{"session", "", &credentials.Credential{Kind: credentials.KindSession, AccessToken: "jwt", Issuer: "https://idp", Email: "m@x"},
			authStatus{Authenticated: true, Method: "session", Storage: "file", Path: "FILE", Issuer: "https://idp", Account: "m@x"}},
		{"env beats stored", validKey, &credentials.Credential{Kind: credentials.KindSession, AccessToken: "jwt"},
			authStatus{Authenticated: true, Method: "api-key", Storage: "environment"}},
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

			stdout, err := run(t, opts, "", "status", "--no-verify")
			require.NoError(t, err)

			var got authStatus
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

// The stored credential is described without opening the secret store:
// --no-verify must never prompt for 1Password.
func TestStatus_ShowsReferenceWithoutResolvingIt(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
	calls := stubOp(t, validKey, false)
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindAPIKeyRef, Ref: "op://Vault/admiral/key",
	}))

	stdout, err := run(t, opts, "", "status", "--no-verify")
	require.NoError(t, err)
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.True(t, got.Authenticated)
	require.Equal(t, "api-key", got.Method)
	require.Equal(t, "1password", got.Storage)
	require.Equal(t, "op://Vault/admiral/key", got.Ref)
	require.Empty(t, got.Path)
	require.False(t, got.Verified)
	require.Equal(t, 0, calls(), "--no-verify opened the secret store")
}

// stubVerify answers for the server: user when err is nil, else err.
func stubVerify(t *testing.T, user *userv1.User, err error) {
	t.Helper()
	prev := verifyIdentity
	verifyIdentity = func(context.Context, *client.Options) (*userv1.User, error) { return user, err }
	t.Cleanup(func() { verifyIdentity = prev })
}

func storeKey(t *testing.T) *client.Options {
	t.Helper()
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON, ServerAddr: "api.example:443"}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{Kind: credentials.KindAPIKey, APIKey: validKey}))
	return opts
}

func TestStatus_VerifiesWithServer(t *testing.T) {
	opts := storeKey(t)
	name := "Martin"
	stubVerify(t, &userv1.User{Id: "u-1", Email: "m@x", DisplayName: &name}, nil)

	stdout, err := run(t, opts, "", "status")
	require.NoError(t, err)
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.True(t, got.Authenticated)
	require.True(t, got.Verified)
	require.Equal(t, &identity{Email: "m@x", DisplayName: "Martin", ID: "u-1"}, got.User)
	require.Empty(t, got.Error)

	// Human mode shows the identity the server resolved.
	opts.OutputFormat = output.FormatTable
	stdout, err = run(t, opts, "", "status")
	require.NoError(t, err)
	require.Contains(t, stdout, "Authenticated:  yes")
	require.Contains(t, stdout, "Email:         m@x")
	require.Contains(t, stdout, "ID:            u-1")
}

// A credential the server refuses is reported as not authenticated, exit
// 4, even though something is stored.
func TestStatus_ServerRejectsCredential(t *testing.T) {
	opts := storeKey(t)
	stubVerify(t, nil, status.Error(codes.Unauthenticated, "key revoked"))

	stdout, err := run(t, opts, "", "status")
	require.NoError(t, err, "-o json: the document carries the outcome")
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.False(t, got.Authenticated)
	require.False(t, got.Verified)
	require.Equal(t, "not signed in: key revoked", got.Error)

	opts.OutputFormat = output.FormatTable
	stdout, err = run(t, opts, "", "status")
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err), "the root maps this to exit 4")
	require.Contains(t, stdout, "Authenticated:  no")
	require.Contains(t, stdout, "key revoked")
}

// A server that cannot be reached says nothing about the credential: it
// stays authenticated, unverified, and human mode exits non-zero with the
// transport error rather than claiming a sign-in problem.
func TestStatus_ServerUnreachable(t *testing.T) {
	opts := storeKey(t)
	stubVerify(t, nil, status.Error(codes.Unavailable, "connection refused"))

	stdout, err := run(t, opts, "", "status")
	require.NoError(t, err)
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.True(t, got.Authenticated)
	require.False(t, got.Verified)
	require.Contains(t, got.Error, "could not reach the Admiral API")

	opts.OutputFormat = output.FormatTable
	stdout, err = run(t, opts, "", "status")
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.NotEqual(t, cmderr.ExitAuth, cmderr.Code(err))
	require.Contains(t, stdout, "Verified:       no (could not reach")
}

func TestStatus_NoVerifySkipsTheServer(t *testing.T) {
	opts := storeKey(t)
	stubVerify(t, nil, errors.New("must not be called"))

	stdout, err := run(t, opts, "", "status", "--no-verify")
	require.NoError(t, err)
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.True(t, got.Authenticated)
	require.False(t, got.Verified)
	require.Empty(t, got.Error)
}

// `admiral whoami` still works for one release: same output, plus a
// deprecation note on stderr so stdout stays parseable.
func TestWhoami_IsDeprecatedAliasOfStatus(t *testing.T) {
	opts := storeKey(t)
	stubVerify(t, &userv1.User{Id: "u-1", Email: "m@x"}, nil)

	cmd := NewWhoamiCmd(opts)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())

	var got authStatus
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.True(t, got.Verified)
	require.Equal(t, "m@x", got.User.Email)
	require.Contains(t, errOut.String(), "deprecated")
	require.True(t, cmd.Hidden, "must not appear in help")
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
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, opts, validKey+"\n", "login", "--with-token", "--scope", "app:read")
	require.ErrorContains(t, err, "--scope applies to browser sign-in")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestStatus_ShowsScopes(t *testing.T) {
	os.Unsetenv(credentials.EnvAPIKey)
	opts := &client.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
	require.NoError(t, credentials.Save(opts.ConfigDir, &credentials.Credential{
		Kind: credentials.KindSession, AccessToken: "jwt", Scopes: []string{"app:read"},
	}))

	stdout, err := run(t, opts, "", "status", "--no-verify")
	require.NoError(t, err)
	var got authStatus
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []string{"app:read"}, got.Scopes)
}
