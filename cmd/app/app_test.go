package app

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
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
		switch verb {
		case "update":
			args = append(args, "--description", "x")
		case "delete":
			args = append(args, "--force") // a piped run cannot confirm
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

// A piped run without --force is refused as a usage error before any
// sign-in or RPC, so a CI job that forgot the flag fails fast with exit 2
// rather than exit 4 for not being signed in.
func TestDeleteWithoutForceFailsBeforeNetwork(t *testing.T) {
	_, err := run(t, "delete", "shop")
	require.EqualError(t, err, "--force required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

// --all walks every page and contradicts --page-token; the refusal is a
// usage error before any network use.
func TestListAllExcludesPageToken(t *testing.T) {
	_, err := run(t, "list", "--all", "--page-token", "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "[all page-token]")

	_, err = run(t, "list", "--all")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "--all alone proceeds to the client")
}
