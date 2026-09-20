package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
)

func run(t *testing.T, opts *admiralclient.Options, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewConfigCmd(opts).Cmd
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestSetGetUnset_RoundTrip(t *testing.T) {
	opts := &admiralclient.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatTable}

	stdout, _, err := run(t, opts, "set", "server", "staging.example:443")
	require.NoError(t, err)
	require.Equal(t, "server: staging.example:443\n", stdout)

	stdout, _, err = run(t, opts, "get", "server")
	require.NoError(t, err)
	require.Equal(t, "server: staging.example:443\n", stdout)

	stdout, _, err = run(t, opts, "unset", "server")
	require.NoError(t, err)
	require.Equal(t, "server: api.admiral.io:443\n", stdout, "unset prints the default that applies again")

	stdout, _, err = run(t, opts, "get", "server")
	require.NoError(t, err)
	require.Equal(t, "server: api.admiral.io:443\n", stdout)
}

// list reports every key with its effective value and where it came from;
// -o json is one document keyed by setting.
func TestList_ReportsSources(t *testing.T) {
	opts := &admiralclient.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatJSON}
	_, _, err := run(t, opts, "set", "output", "json")
	require.NoError(t, err)

	stdout, _, err := run(t, opts, "list")
	require.NoError(t, err)
	var got map[string]settingJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, settingJSON{Value: "json", Source: "config file"}, got["output"])
	require.Equal(t, settingJSON{Value: "api.admiral.io:443", Source: "default"}, got["server"])

	opts.OutputFormat = output.FormatTable
	stdout, stderr, err := run(t, opts, "list")
	require.NoError(t, err)
	require.Contains(t, stdout, "KEY")
	require.Contains(t, stdout, "config file")
	require.Contains(t, stderr, "Config file:", "the path is messaging, not the answer")
}

// Unknown keys, values a key does not accept, and bad argument counts are
// usage errors (exit 2).
func TestUsageErrors(t *testing.T) {
	opts := &admiralclient.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatTable}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get", "bogus"}, `unknown config key "bogus"`},
		{[]string{"set", "bogus", "x"}, `unknown config key "bogus"`},
		{[]string{"unset", "bogus"}, `unknown config key "bogus"`},
		{[]string{"set", "insecure", "yes"}, `invalid value "yes" for insecure: must be true or false`},
		{[]string{"set"}, "missing argument: set <key> [value]"},
		{[]string{"set", "server", "a", "b"}, "accepts at most 2 arg(s), received 3"},
		{[]string{"get"}, "missing argument: get <key>"},
		{[]string{"list", "extra"}, `unknown argument "extra"`},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := run(t, opts, tc.args...)
			require.ErrorContains(t, err, tc.want)
			require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		})
	}
}

// Omitting the value prompts, and a prompt cannot be shown in a pipe.
func TestSet_PromptNeedsATerminal(t *testing.T) {
	opts := &admiralclient.Options{ConfigDir: t.TempDir(), OutputFormat: output.FormatTable}
	_, _, err := run(t, opts, "set", "server")
	require.ErrorContains(t, err, "cannot prompt for server when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}
