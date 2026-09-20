package component

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewComponentCmd(&client.Options{})
	var out, errOut bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&errOut)
	root.Cmd.SetArgs(args)
	err := root.Cmd.Execute()
	return out.String() + errOut.String(), err
}

// Missing or extra positionals are usage errors (exit 2) with a hint to
// --help; the full usage block is never printed.
func TestPositionalUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get"}, "missing argument: get <name>"},
		{[]string{"list", "extra-arg"}, `unknown argument "extra-arg"`},
		{[]string{"publish", "a", "b"}, `accepts at most 1 arg(s), received 2`},
		{[]string{"tag", "--digest", "sha256:ab"}, "missing argument: tag <name>:<tag> --digest <digest>"},
		{[]string{"deprecate", "--digest", "sha256:ab", "--reason", "x"}, "missing argument: deprecate <name> --digest <digest> --reason <text>"},
		{[]string{"untag"}, "missing argument: untag <name>:<tag>"},
		{[]string{"revision", "list"}, "missing argument: list <name>"},
		{[]string{"revision", "get"}, "missing argument: get <name>:<tag> | <name>@<digest>"},
	}
	for _, tc := range cases {
		t.Run(tc.args[0], func(t *testing.T) {
			printed, err := run(t, tc.args...)
			require.EqualError(t, err, tc.want)
			assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
			assert.Empty(t, printed)
		})
	}
}

// Publish fails before any network for what it can tell locally: a name
// the registry would refuse, a directory that is not a component, a kind
// the flag does not know.
func TestPublishRefusesLocallyWhatItCan(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`variable "x" {}`), 0o644))

	_, err := run(t, "publish", dir, "--name", "Not_Valid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase letters, digits and hyphens")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "publish", t.TempDir(), "--name", "empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot tell what kind")

	_, err = run(t, "publish", dir, "--kind", "docker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker")
}

func TestFormatting(t *testing.T) {
	assert.Equal(t, "3f9a2c1d3f9a", shortDigest("sha256:3f9a2c1d3f9a2c1d3f9a2c1d3f9a2c1d3f9a2c1d3f9a2c1d3f9a2c1d3f9a2c1d"))
	assert.Equal(t, "512 B", formatSize(512))
	assert.Equal(t, "4 KiB", formatSize(4096))
	assert.Equal(t, "1.5 MiB", formatSize(3<<19))
	assert.Equal(t, "64 MiB", formatSize(64<<20))
}

// A reference is NAME:TAG, NAME@DIGEST or a bare NAME; the verbs that need
// the tag or digest half refuse a bare name before any network.
func TestSplitRef(t *testing.T) {
	cases := []struct{ in, name, ref string }{
		{"cloud-sql", "cloud-sql", ""},
		{"cloud-sql:v1.2.0", "cloud-sql", "v1.2.0"},
		{"cloud-sql:latest", "cloud-sql", "latest"},
		{"cloud-sql@sha256:3f9a", "cloud-sql", "sha256:3f9a"},
	}
	for _, tc := range cases {
		name, ref, err := splitRef(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.name, name, tc.in)
		assert.Equal(t, tc.ref, ref, tc.in)
	}
	for _, bad := range []string{":v1", "@sha256:ab", "cloud-sql:", "cloud-sql@"} {
		_, _, err := splitRef(bad)
		require.Error(t, err, bad)
		assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err), bad)
	}

	_, err := run(t, "tag", "cloud-sql", "--digest", "sha256:ab")
	require.EqualError(t, err, `"cloud-sql" names no tag; use <name>:<tag>`)
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "untag", "cloud-sql")
	require.EqualError(t, err, `"cloud-sql" names no tag; use <name>:<tag>`)
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "revision", "get", "cloud-sql")
	require.EqualError(t, err, `"cloud-sql" names no revision; use <name>:<tag> or <name>@<digest>`)
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	// --status is an enum flag: the flag itself refuses the value, before
	// RunE, and completes from the same list.
	_, err = run(t, "revision", "list", "cloud-sql", "--status", "retired")
	require.ErrorContains(t, err, "must be one of published, deprecated")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}
