package env

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// run executes the env command tree with no stored credential, so anything
// that gets past argument handling fails at client creation (exit 4), which
// tells a usage error apart from a command that was accepted.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	os.Unsetenv(credentials.EnvAPIKey)
	root := NewEnvCmd(&client.Options{ConfigDir: t.TempDir()})
	var out, errOut bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&errOut)
	root.Cmd.SetArgs(args)
	err := root.Cmd.Execute()
	return out.String() + errOut.String(), err
}

const envID = "env-3k9x2m7qpl0a"

// Missing or extra positionals are usage errors (exit 2) with a hint to
// --help; the full usage block is never printed.
func TestPositionalUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"create"}, "missing argument: create <name>"},
		{[]string{"get"}, "missing argument: get <name>"},
		{[]string{"describe"}, "missing argument: describe <name>"},
		{[]string{"update"}, "missing argument: update <name>"},
		{[]string{"delete"}, "missing argument: delete <name>"},
		{[]string{"list", "a", "b"}, "accepts at most 1 arg(s), received 2"},
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

// An environment is addressed as app/env or as a name with --app, never
// both, and the rule is enforced by every verb that takes one.
func TestPathAndAppFlagIsUsageError(t *testing.T) {
	for _, verb := range []string{"get", "describe", "delete", "create"} {
		t.Run(verb, func(t *testing.T) {
			_, err := run(t, verb, "shop/prod", "--app", "shop")
			require.EqualError(t, err, "--app cannot be combined with a path")
			require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
			require.Equal(t, "Give the environment as app/env or with --app, not both.", cmderr.Hint(err))
		})
	}
	_, err := run(t, "update", "shop/prod", "--app", "shop", "--description", "x")
	require.EqualError(t, err, "--app cannot be combined with a path")
}

func TestMalformedPathIsUsageError(t *testing.T) {
	for _, target := range []string{"/prod", "shop/", "a/b/c"} {
		_, err := run(t, "get", target)
		require.EqualError(t, err, `invalid environment "`+target+`": expected app/env`)
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	}
}

// list is scoped by an application named positionally or with --app; the
// missing scope is reported before any client is built.
func TestList_ScopeRules(t *testing.T) {
	_, err := run(t, "list")
	require.EqualError(t, err, "no application specified")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Contains(t, cmderr.Hint(err), "admiral env list billing")

	_, err = run(t, "list", "billing", "--app", "other")
	require.EqualError(t, err, "--app cannot be combined with an application argument")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "list", "shop/prod")
	require.EqualError(t, err, `invalid application "shop/prod"`)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Contains(t, cmderr.Hint(err), "app/env addresses a single environment")

	// A well-formed scope proceeds to the client.
	for _, args := range [][]string{{"list", "billing"}, {"list", "--app", "billing"}} {
		_, err = run(t, args...)
		require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
	}
}

// A path, a --app-scoped name, or a bare id is accepted as the identity
// and the command proceeds to client creation.
func TestWellFormedTargetsAreAccepted(t *testing.T) {
	for _, verb := range []string{"get", "describe", "delete"} {
		for _, args := range [][]string{{verb, "shop/prod"}, {verb, "prod", "--app", "shop"}, {verb, envID}} {
			if verb == "delete" {
				args = append(args, "--yes") // a piped run cannot confirm
			}
			_, err := run(t, args...)
			require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
		}
	}
	_, err := run(t, "update", envID, "--description", "x")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated)
	_, err = run(t, "create", "shop/staging")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated)
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	_, err := run(t, "update", "shop/prod")
	require.ErrorContains(t, err, "at least one of --name, --description, --label, --agent, --namespace or --approvals")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestCreate_RejectsBadLabel(t *testing.T) {
	_, err := run(t, "create", "shop/staging", "--label", "novalue")
	require.ErrorContains(t, err, `invalid label format "novalue"`)
}

// A piped run without --yes is refused before any sign-in or RPC, but
// a malformed target is reported first.
func TestDeleteWithoutYesFailsBeforeNetwork(t *testing.T) {
	_, err := run(t, "delete", "shop/prod")
	require.EqualError(t, err, "--yes required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "delete", "shop/prod", "--app", "shop")
	require.EqualError(t, err, "--app cannot be combined with a path")
}

func TestTargetFlags(t *testing.T) {
	for _, verb := range []string{"create", "update"} {
		_, err := run(t, verb, "shop/prod", "--agent", "prod", "--namespace", "Shop")
		require.EqualError(t, err, `invalid namespace "Shop": at most 63 lowercase letters, digits and hyphens`)
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

		_, err = run(t, verb, "shop/prod", "--agent", "a", "--agent", "b", "--namespace", "shop")
		require.EqualError(t, err, "--namespace needs exactly one --agent, got 2")
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

		_, err = run(t, verb, "shop/prod", "--approvals", "-1")
		require.EqualError(t, err, "--approvals cannot be negative")

		for _, args := range [][]string{
			{verb, "shop/prod", "--agent", "prod-cluster"},
			{verb, "shop/prod", "--agent", "agt-3k9x2m7qpl0a", "--namespace", "shop-prod"},
			{verb, "shop/prod", "--approvals", "2"},
		} {
			_, err = run(t, args...)
			require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
		}
	}
	// Create has no assignment to put a namespace on without --agent.
	_, err := run(t, "create", "shop/prod", "--namespace", "shop")
	require.EqualError(t, err, "--namespace needs exactly one --agent, got 0")
}

// Only the flags given are written, each names its mask path, and an agent
// that stays keeps its assignment and settings. Agents are given as ids so
// no lookup is made.
func TestTargetFlagsApply(t *testing.T) {
	const kept, added = "agt-aaaaaaaaaaaa", "agt-bbbbbbbbbbbb"
	parse := func(e *environmentv1.Environment, args ...string) ([]string, error) {
		cmd := &cobra.Command{Use: "x"}
		var f targetFlags
		f.register(cmd, "", "")
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, f.check(cmd, false))
		return f.apply(context.Background(), cmd, nil, e)
	}
	current := func() *environmentv1.Environment {
		return &environmentv1.Environment{Agents: []*environmentv1.AgentAssignment{{
			AgentId: kept, Id: "asg-111111111111",
			Settings: &environmentv1.AgentAssignment_Kubernetes{Kubernetes: &environmentv1.KubernetesSettings{DefaultNamespace: "shop"}},
		}}}
	}

	e := current()
	paths, err := parse(e)
	require.NoError(t, err)
	require.Empty(t, paths)

	e = current()
	paths, err = parse(e, "--agent", added, "--agent", kept, "--agent", added)
	require.NoError(t, err)
	require.Equal(t, []string{"agents"}, paths)
	require.Len(t, e.Agents, 2, "a repeated agent is assigned once")
	require.Equal(t, added, e.Agents[0].AgentId)
	require.Empty(t, e.Agents[0].Id, "a new assignment gets its id from the server")
	require.Equal(t, "asg-111111111111", e.Agents[1].Id)
	require.Equal(t, "shop", e.Agents[1].GetKubernetes().GetDefaultNamespace())

	e = current()
	paths, err = parse(e, "--namespace", "shop-prod", "--approvals", "2")
	require.NoError(t, err)
	require.Equal(t, []string{"agents", "approval"}, paths)
	require.Equal(t, "shop-prod", e.Agents[0].GetKubernetes().GetDefaultNamespace())
	require.EqualValues(t, 2, e.Approval.RequiredApprovals)

	e = &environmentv1.Environment{}
	_, err = parse(e, "--namespace", "shop")
	require.EqualError(t, err, "--namespace sets the namespace of an environment's one agent; this one has 0")
}

func TestDescribeEnv(t *testing.T) {
	yes := true
	e := &environmentv1.Environment{
		Name: "prod", Id: envID, Generation: 7,
		Agents: []*environmentv1.AgentAssignment{{
			AgentId: "agt-aaaaaaaaaaaa", Id: "asg-111111111111", ApplyProfile: "edit",
			Settings: &environmentv1.AgentAssignment_Kubernetes{Kubernetes: &environmentv1.KubernetesSettings{DefaultNamespace: "shop-prod", Prune: &yes}},
		}},
		Approval: &environmentv1.ApprovalRule{RequiredApprovals: 2},
		Hold:     &environmentv1.Hold{ChangeSetId: "cs-7q2k9m4x8p1c", HeldAt: timestamppb.New(time.Now().Add(-3 * time.Hour))},
	}
	sec := envSections{
		agentNames: map[string]string{"agt-aaaaaaaaaaaa": "prod-cluster"},
		comps: []*environmentv1.EnvironmentComponent{{
			Name: "api", Version: &changesetv1.VersionRef{Reference: "v1.2.0", Digest: "sha256:abc"},
			LastApply: &environmentv1.LastApply{AppliedAt: timestamppb.New(time.Now().Add(-2 * time.Hour))},
		}},
	}
	var b bytes.Buffer
	require.NoError(t, describeEnv(e, &applicationv1.Application{Name: "shop"}, sec).Render(&b))
	out := b.String()
	require.Regexp(t, `Generation:\s+7`, out)
	require.Regexp(t, `Hold:\s+held by cs-7q2k9m4x8p1c since 3h ago`, out)
	require.Regexp(t, `prod-cluster\s+asg-111111111111\s+default\s+edit\s+shop-prod\s+<none>\s+true`, out)
	require.Regexp(t, `Required Approvals:\s+2`, out)
	require.Regexp(t, `api\s+<none>\s+v1.2.0\s+120m ago`, out)
}
