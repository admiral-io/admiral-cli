package app

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
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

// An id positional is accepted as the identity without any --id flag; the
// command proceeds to client creation.
func TestIDPositionalIsAccepted(t *testing.T) {
	for _, verb := range []string{"get", "delete", "update"} {
		args := []string{verb, "app-7q2k9m4x8p1c"}
		switch verb {
		case "update":
			args = append(args, "--description", "x")
		case "delete":
			args = append(args, "--yes") // a piped run cannot confirm
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

// A piped run without --yes is refused as a usage error before any
// sign-in or RPC, so a CI job that forgot the flag fails fast with exit 2
// rather than exit 4 for not being signed in.
func TestDeleteWithoutYesFailsBeforeNetwork(t *testing.T) {
	_, err := run(t, "delete", "shop")
	require.EqualError(t, err, "--yes required when not running interactively")
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

func TestEnvironmentRowsSayWhatRunsWhere(t *testing.T) {
	ago := timestamppb.New(time.Now().Add(-39 * time.Minute))
	envs := []*environmentv1.Environment{
		{Id: "env-dev000000001", Name: "dev", LastApply: &environmentv1.LastApply{ChangeSetId: "cs-aaaaaaaaaaaa", AppliedAt: ago}},
		{Id: "env-prod00000001", Name: "prod", LastApply: &environmentv1.LastApply{ChangeSetId: "cs-bbbbbbbbbbbb", AppliedAt: ago},
			Hold: &environmentv1.Hold{ChangeSetId: "cs-cccccccccccc"}},
		{Id: "env-stage0000001", Name: "staging"},
	}
	comps := []*environmentv1.EnvironmentComponent{
		{EnvironmentId: "env-dev000000001", Name: "podinfo", Version: &changesetv1.VersionRef{Reference: "6.8.0", Digest: "sha256:3902c9ddbe75aaaa"}},
		{EnvironmentId: "env-prod00000001", Name: "podinfo", Version: &changesetv1.VersionRef{VersionId: "ver-x", Digest: "sha256:d0659ccea357aaaa"}},
	}
	runs := []*runv1.Run{{EnvironmentId: "env-dev000000001", Status: runv1.Run_STATUS_APPLYING}}

	rows := environmentRows(envs, comps, nil, map[string][]string{"ver-x": {"6.7.1"}}, runs)
	require.Equal(t, [][]string{
		{"dev", "podinfo 6.8.0 (sha256:3902c9ddbe75)", "cs-aaaaaaaaaaaa 39m ago", "applying"},
		{"prod", "podinfo 6.7.1 (sha256:d0659ccea357)", "cs-bbbbbbbbbbbb 39m ago", "held by cs-cccccccccccc"},
		{"staging", "", "", "never applied"},
	}, rows)

	rows = environmentRows(envs[:1], nil, errors.New("denied"), nil, nil)
	require.Equal(t, "<unavailable>", rows[0][1])
}
