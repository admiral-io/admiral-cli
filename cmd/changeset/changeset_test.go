package changeset

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// run executes the changeset tree with no stored credential, so anything
// that gets past argument handling fails at client creation, which tells a
// usage error apart from a command that was accepted.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	os.Unsetenv(credentials.EnvAPIKey)
	root := NewChangeSetCmd(&client.Options{ConfigDir: t.TempDir()})
	var out, errOut bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&errOut)
	root.Cmd.SetArgs(args)
	err := root.Cmd.Execute()
	return out.String() + errOut.String(), err
}

const (
	csID = "cs-7f2a1c9d0e3b"
	uuid = "550e8400-e29b-41d4-a716-446655440000"
)

func requireUsage(t *testing.T, err error, contains string) {
	t.Helper()
	require.ErrorContains(t, err, contains)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err), err.Error())
}

func requireAccepted(t *testing.T, err error, args ...string) {
	t.Helper()
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
}

func TestPositionalUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"create"}, "missing argument: create <app>/<env>"},
		{[]string{"get"}, "missing argument: get <change-set>"},
		{[]string{"discard"}, "missing argument: discard <change-set>"},
		{[]string{"list", "shop/prod"}, `unknown argument "shop/prod"`},
		{[]string{"bogus"}, `unknown command "bogus" for "changeset"`},
	}
	for _, tc := range cases {
		t.Run(tc.args[0], func(t *testing.T) {
			printed, err := run(t, tc.args...)
			require.EqualError(t, err, tc.want)
			require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
			require.Contains(t, cmderr.Hint(err), "--help")
			require.NotContains(t, printed, "Usage:", "usage block must not be dumped")
		})
	}
}

// A change set is addressed by its cs- ID only; anything else is refused
// before any sign-in.
func TestChangeSetIDIsCheckedBeforeTheNetwork(t *testing.T) {
	for _, bad := range []string{"cs-7F2A1C9D0E3B", "cs-7f2a1", "7f2a1c9d0e3b", uuid} {
		_, err := run(t, "get", bad)
		requireUsage(t, err, `invalid change set "`+bad+`"`)
		require.Contains(t, cmderr.Hint(err), "changeset list --env")

		_, err = run(t, "discard", bad, "--force")
		requireUsage(t, err, `invalid change set "`+bad+`"`)
	}
	_, err := run(t, "get", csID)
	requireAccepted(t, err, "get", csID)
}

// create takes the environment as app/env or a UUID; a bare name has no
// application to look it up in.
func TestCreate_NeedsAnEnvironmentPath(t *testing.T) {
	_, err := run(t, "create", "prod")
	requireUsage(t, err, "no application specified")
	require.Contains(t, cmderr.Hint(err), "shop/prod")

	_, err = run(t, "create", "shop/")
	requireUsage(t, err, `invalid environment "shop/": expected app/env`)

	for _, target := range []string{"shop/prod", uuid} {
		_, err = run(t, "create", target, "--title", "bump api")
		requireAccepted(t, err, "create", target)
	}
}

// --set is parsed in full before the client is built, so a typo never
// half-applies and never costs a sign-in.
func TestCreate_SetIsParsedBeforeTheNetwork(t *testing.T) {
	cases := []struct{ flag, set, want string }{
		{"--set", "api.image.tag", "is not component.path=value"},
		{"--set", "api=1", "names no path inside the component"},
		{"--set", "api.image..tag=v2", "has an empty key"},
		{"--set", "Api.replicas=3", `invalid component name "Api"`},
		{"--set", "api.replicas=[3", "api.replicas"},
		{"--set-string", "api.image.tag", "is not component.path=value"},
	}
	for _, tc := range cases {
		t.Run(tc.set, func(t *testing.T) {
			_, err := run(t, "create", "shop/prod", tc.flag, tc.set)
			requireUsage(t, err, tc.want)
		})
	}

	args := []string{"create", "shop/prod", "--set", "api.replicas=3", "--set-string", "api.build=0042"}
	_, err := run(t, args...)
	requireAccepted(t, err, args...)
}

func TestList_RequiresAnEnvironmentPath(t *testing.T) {
	_, err := run(t, "list")
	requireUsage(t, err, "no environment specified")
	require.Contains(t, cmderr.Hint(err), "--env shop/prod")

	_, err = run(t, "list", "--env", "prod")
	requireUsage(t, err, "no application specified")

	_, err = run(t, "list", "--env", "shop/prod", "--status", "merged")
	requireUsage(t, err, "must be one of draft, discarded")

	for _, args := range [][]string{
		{"list", "--env", "shop/prod"},
		{"list", "--env", uuid, "--status", "draft"},
	} {
		_, err = run(t, args...)
		requireAccepted(t, err, args...)
	}
}

// discard is the moderate confirmation tier: a piped run needs --force,
// and says so before any sign-in.
func TestDiscardWithoutForceFailsBeforeNetwork(t *testing.T) {
	_, err := run(t, "discard", csID)
	require.EqualError(t, err, "--force required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	for _, force := range []string{"--force", "-f"} {
		_, err = run(t, "discard", csID, force)
		requireAccepted(t, err, "discard", csID, force)
	}
}

func keys(segs []*changesetv1.PathSegment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.GetKey()
	}
	return out
}

// A --set value reads as a YAML scalar, as helm --set does; --set-string
// keeps the characters; null is SetNull, never the string "null".
func TestSetEdit_ReadsValuesLikeHelm(t *testing.T) {
	cases := []struct {
		raw         string
		forceString bool
		want        string // value_json; "" means SetNull
	}{
		{"3", false, `3`},
		{"3", true, `"3"`},
		{`"3"`, false, `"3"`},
		{"true", false, `true`},
		{"true", true, `"true"`},
		{"1.50", false, `1.50`},
		{"12345678901234567890", false, `12345678901234567890`},
		{"v1.4.0", false, `"v1.4.0"`},
		{"", false, `""`},
		{"!ref users-db.host", false, `{"$ref":"users-db.host"}`},
		{"!ref users-db.host", true, `"!ref users-db.host"`},
		{"null", true, `"null"`},
		{"null", false, ""},
		{"~", false, ""},
	}
	for _, tc := range cases {
		a := valuesfile.Assignment{Component: "api", Path: []string{"image", "tag"}, Raw: tc.raw}
		e, err := setEdit(a, tc.forceString)
		require.NoError(t, err, tc.raw)
		if tc.want == "" {
			sn := e.GetSetNull()
			require.NotNil(t, sn, "%q should be SetNull", tc.raw)
			assert.Equal(t, "api", sn.Component)
			assert.Equal(t, []string{"image", "tag"}, keys(sn.Path))
			continue
		}
		sv := e.GetSetValue()
		require.NotNil(t, sv, "%q should be SetValue", tc.raw)
		assert.Equal(t, "api", sv.Component)
		assert.Equal(t, []string{"image", "tag"}, keys(sv.Path))
		assert.Equal(t, tc.want, sv.ValueJson, "raw %q, forceString %v", tc.raw, tc.forceString)
	}
}

// A dot inside a key survives as one segment, and --set-string lands after
// --set, so the later edit wins at the same path.
func TestSetEdits_PathsAndOrder(t *testing.T) {
	es, err := setEdits(
		[]string{`api.annotations."nginx.io/rewrite"=/`, `api.replicas=3`},
		[]string{`api.replicas=3`},
	)
	require.NoError(t, err)
	require.Len(t, es, 3)
	assert.Equal(t, []string{"annotations", "nginx.io/rewrite"}, keys(es[0].GetSetValue().Path))
	assert.Equal(t, `3`, es[1].GetSetValue().ValueJson)
	assert.Equal(t, `"3"`, es[2].GetSetValue().ValueJson)
}

func TestPathSegments(t *testing.T) {
	assert.Empty(t, pathSegments(nil))
	assert.Equal(t, []string{"a", "b.c", ""}, keys(pathSegments([]string{"a", "b.c", ""})))
}
