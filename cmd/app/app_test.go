package app

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewAppCmd(&client.Options{})
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
		{[]string{"create"}, "missing argument: create <name>"},
		{[]string{"get"}, "missing argument: get <name>"},
		{[]string{"update"}, "missing argument: update <name>"},
		{[]string{"delete"}, "missing argument: delete <name>"},
		{[]string{"list", "extra-arg"}, `unknown argument "extra-arg"`},
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

// A UUID positional is accepted as the identity without any --id flag; the
// command proceeds to client creation.
func TestUUIDPositionalIsAccepted(t *testing.T) {
	for _, verb := range []string{"get", "delete", "update"} {
		args := []string{verb, "550e8400-e29b-41d4-a716-446655440000"}
		if verb == "update" {
			args = append(args, "--description", "x")
		}
		_, err := run(t, args...)
		require.Error(t, err)
		require.NotEqual(t, cmderr.ExitUsage, cmderr.Code(err), "%s: should fail at client creation, not usage", verb)
	}
}

func TestIDFlagIsGone(t *testing.T) {
	_, err := run(t, "get", "--id", "x")
	require.ErrorContains(t, err, "unknown flag: --id")
}

func TestUpdateCmd_RequiresAtLeastOneField(t *testing.T) {
	_, err := run(t, "update", "billing-api")
	require.ErrorContains(t, err, "at least one of --name, --label, or --description")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}
