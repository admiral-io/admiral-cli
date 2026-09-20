package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.admiral.io/sdk/client"
)

// fakeOp installs a stub `op` on PATH and routes lookups through an
// in-process function returning out or err, recording each call.
func fakeOp(t *testing.T, out string, err error) *[][]string {
	t.Helper()
	bin := t.TempDir()
	if werr := os.WriteFile(filepath.Join(bin, "op"), []byte("#!/bin/sh\nexit 0\n"), 0755); werr != nil { //nolint:gosec // test stub must be executable
		t.Fatal(werr)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var calls [][]string
	prev := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if err != nil {
			return nil, err
		}
		return []byte(out), nil
	}
	t.Cleanup(func() { runCommand = prev })
	return &calls
}

// TestRunCommand_RealExec covers the production exec path with a script,
// including stderr capture on failure.
func TestRunCommand_RealExec(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = fail ]; then echo boom >&2; exit 1; fi\nprintf '%s' \"$2\"\n"
	if err := os.WriteFile(filepath.Join(bin, "stub"), []byte(script), 0755); err != nil { //nolint:gosec // test stub
		t.Fatal(err)
	}

	out, err := runCommand(context.Background(), filepath.Join(bin, "stub"), "ok", "value")
	if err != nil || string(out) != "value" {
		t.Fatalf("got %q, %v", out, err)
	}

	_, err = runCommand(context.Background(), filepath.Join(bin, "stub"), "fail")
	if err == nil || err.Error() != filepath.Join(bin, "stub")+": boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsRef(t *testing.T) {
	require.True(t, IsRef("op://Vault/item/field"))
	require.True(t, IsRef("keychain://admiral/key"))
	require.False(t, IsRef("admp_abc"))
	require.False(t, IsRef("://nothing"))
	require.False(t, IsRef("has space://x"))
	require.False(t, IsRef(""))
}

func TestResolveRef_1Password(t *testing.T) {
	calls := fakeOp(t, "admp_secret\n", nil)

	got, err := ResolveRef(context.Background(), "op://Vault/item/field")
	require.NoError(t, err)
	require.Equal(t, "admp_secret", got)
	require.Equal(t, [][]string{{"op", "read", "--no-newline", "op://Vault/item/field"}}, *calls)
}

func TestResolveRef_1PasswordError(t *testing.T) {
	fakeOp(t, "", errors.New("op: [ERROR] item not found"))

	_, err := ResolveRef(context.Background(), "op://Vault/missing/field")
	require.ErrorContains(t, err, "op://Vault/missing/field")
	require.ErrorContains(t, err, "item not found")
}

func TestResolveRef_1PasswordEmpty(t *testing.T) {
	fakeOp(t, "  \n", nil)

	_, err := ResolveRef(context.Background(), "op://Vault/item/field")
	require.ErrorContains(t, err, "empty value")
}

func TestResolveRef_1PasswordNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing on PATH

	_, err := ResolveRef(context.Background(), "op://Vault/item/field")
	require.ErrorContains(t, err, "not found in PATH")
}

func TestResolveRef_UnsupportedScheme(t *testing.T) {
	_, err := ResolveRef(context.Background(), "vault://secret/admiral")
	require.ErrorContains(t, err, `unsupported credential reference scheme "vault"`)
}

func TestResolveRef_NotARef(t *testing.T) {
	_, err := ResolveRef(context.Background(), "admp_literal")
	require.ErrorContains(t, err, "not a credential reference")
}

func TestResolveToken_StoredRef(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	calls := fakeOp(t, "admp_from_op", nil)
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKeyRef, Ref: "op://Vault/item/field"}))

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "admp_from_op", got.Token)
	require.Equal(t, client.AuthSchemeToken, got.AuthScheme)
	require.Equal(t, SourceAPIKey, got.Source)
	require.Len(t, *calls, 1)

	// Only the reference is on disk.
	raw, err := os.ReadFile(filepath.Join(dir, credentialsFile))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "admp_from_op")
}

func TestResolveToken_StoredRefFailure(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	fakeOp(t, "", errors.New("vault is locked"))
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKeyRef, Ref: "op://Vault/item/field"}))

	_, err := ResolveToken(context.Background(), dir)
	require.ErrorContains(t, err, "resolving stored credential reference")
	require.ErrorContains(t, err, "vault is locked")
}

func TestResolveToken_StoredRefEmpty(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKeyRef}))

	_, err := ResolveToken(context.Background(), dir)
	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestRefresh_IgnoresStoredRef(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKeyRef, Ref: "op://Vault/item/field"}))

	_, err := ForceRefresh(context.Background(), dir)
	require.Error(t, err)
}

// The real exec path hands the child our stdin, so a store that must prompt
// can read the answer.
func TestRunCommand_ChildInheritsStdin(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nread answer\nprintf '%s' \"got:$answer\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "stub"), []byte(script), 0755)) //nolint:gosec // test stub

	r, w, err := os.Pipe()
	require.NoError(t, err)
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev })
	_, _ = w.WriteString("yes\n")
	_ = w.Close()

	out, err := runCommand(context.Background(), filepath.Join(bin, "stub"))
	require.NoError(t, err)
	require.Equal(t, "got:yes", string(out))
}

// The caller's deadline is what bounds `op read`, not only resolveTimeout:
// shell completion gives itself two seconds and must not sit behind a
// biometric prompt for sixty.
func TestResolveToken_StoredRefReceivesCallerContext(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	fakeOp(t, "admp_from_op", nil)
	var seen context.Context
	prev := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		seen = ctx
		return []byte("admp_from_op"), nil
	}
	t.Cleanup(func() { runCommand = prev })
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKeyRef, Ref: "op://Vault/item/field"}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ResolveToken(ctx, dir)
	require.NoError(t, err)
	require.NotNil(t, seen)
	deadline, ok := seen.Deadline()
	require.True(t, ok, "op read must run under a deadline")
	require.WithinDuration(t, time.Now().Add(time.Second), deadline, 500*time.Millisecond,
		"the caller's one-second deadline wins over the sixty-second default")
}
