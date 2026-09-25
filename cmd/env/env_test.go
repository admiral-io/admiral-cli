package env

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
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

const uuid = "550e8400-e29b-41d4-a716-446655440000"

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

// A path, a --app-scoped name, or a bare UUID is accepted as the identity
// and the command proceeds to client creation.
func TestWellFormedTargetsAreAccepted(t *testing.T) {
	for _, verb := range []string{"get", "describe", "delete"} {
		for _, args := range [][]string{{verb, "shop/prod"}, {verb, "prod", "--app", "shop"}, {verb, uuid}} {
			if verb == "delete" {
				args = append(args, "--force") // a piped run cannot confirm
			}
			_, err := run(t, args...)
			require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
		}
	}
	_, err := run(t, "update", uuid, "--description", "x")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated)
	_, err = run(t, "create", "shop/staging")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated)
}

func TestUpdate_RequiresAtLeastOneField(t *testing.T) {
	_, err := run(t, "update", "shop/prod")
	require.ErrorContains(t, err, "at least one of --name, --description, --label, --namespace or --create-namespaces")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestCreate_RejectsBadLabel(t *testing.T) {
	_, err := run(t, "create", "shop/staging", "--label", "novalue")
	require.ErrorContains(t, err, `invalid label format "novalue"`)
}

// A piped run without --force is refused before any sign-in or RPC, but
// a malformed target is reported first.
func TestDeleteWithoutForceFailsBeforeNetwork(t *testing.T) {
	_, err := run(t, "delete", "shop/prod")
	require.EqualError(t, err, "--force required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "delete", "shop/prod", "--app", "shop")
	require.EqualError(t, err, "--app cannot be combined with a path")
}

func TestKubernetesFlags(t *testing.T) {
	for _, verb := range []string{"create", "update"} {
		_, err := run(t, verb, "shop/prod", "--namespace", "Shop")
		require.EqualError(t, err, `invalid namespace "Shop": at most 63 lowercase letters, digits and hyphens`)
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

		_, err = run(t, verb, "shop/prod", "--create-namespaces=maybe")
		require.ErrorContains(t, err, `invalid argument "maybe"`)

		for _, args := range [][]string{
			{verb, "shop/prod", "--namespace", "shop-prod"},
			{verb, "shop/prod", "--create-namespaces=false"},
			{verb, "shop/prod", "--namespace", "shop", "--create-namespaces=true"},
		} {
			_, err = run(t, args...)
			require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "%v: should fail at client creation, not usage", args)
		}
	}
}

// Only the flags given are written, and each names its mask path; an
// empty namespace is a value, the default.
func TestKubernetesFlagsApply(t *testing.T) {
	parse := func(args ...string) (*environmentv1.KubernetesTarget, []string, error) {
		cmd := &cobra.Command{Use: "x"}
		var k kubernetesFlags
		k.register(cmd, "")
		require.NoError(t, cmd.ParseFlags(args))
		var kt *environmentv1.KubernetesTarget
		paths, err := k.apply(cmd, &kt)
		return kt, paths, err
	}

	kt, paths, err := parse()
	require.NoError(t, err)
	require.Nil(t, kt, "no flag, no target: the server's defaults apply")
	require.Empty(t, paths)

	kt, paths, err = parse("--namespace", "shop-prod")
	require.NoError(t, err)
	require.Equal(t, "shop-prod", kt.Namespace)
	require.Nil(t, kt.CreateNamespaces)
	require.Equal(t, []string{"kubernetes.namespace"}, paths)

	kt, paths, err = parse("--create-namespaces=false")
	require.NoError(t, err)
	require.NotNil(t, kt.CreateNamespaces)
	require.False(t, *kt.CreateNamespaces)
	require.Equal(t, []string{"kubernetes.create_namespaces"}, paths)

	kt, paths, err = parse("--namespace=", "--create-namespaces")
	require.NoError(t, err)
	require.Empty(t, kt.Namespace)
	require.True(t, *kt.CreateNamespaces)
	require.Equal(t, []string{"kubernetes.namespace", "kubernetes.create_namespaces"}, paths)

	_, _, err = parse("--namespace", strings.Repeat("n", 64))
	require.ErrorContains(t, err, "invalid namespace")
}

func TestDescribeKubernetes(t *testing.T) {
	yes := true
	render := func(kt *environmentv1.KubernetesTarget) string {
		d := output.NewDescribe()
		describeKubernetes(d, kt)
		var b bytes.Buffer
		require.NoError(t, d.Render(&b))
		return b.String()
	}

	out := render(&environmentv1.KubernetesTarget{Namespace: "shop-prod", CreateNamespaces: &yes})
	require.Regexp(t, `Namespace:\s+shop-prod`, out)
	require.Regexp(t, `Create Namespaces:\s+true`, out)
	require.Regexp(t, `Capabilities:\s+<none reported>`, out)

	out = render(&environmentv1.KubernetesTarget{Namespace: "shop-prod", Capabilities: &environmentv1.KubernetesCapabilities{
		KubeVersion: "v1.31.2",
		ApiVersions: []string{"v1", "apps/v1", "batch/v1", "batch/v1beta1", "monitoring.coreos.com/v1"},
		ReportedAt:  timestamppb.New(time.Now().Add(-3 * time.Hour)),
	}})
	require.Regexp(t, `Version:\s+v1.31.2`, out)
	require.Regexp(t, `API Groups:\s+4`, out, "core, apps, batch, monitoring.coreos.com")
	require.Regexp(t, `Reported:\s+.*\(3h ago\)`, out)

	require.Equal(t, "v1.31.2, 1 API groups, 3h ago", capabilitiesSummary(&environmentv1.KubernetesCapabilities{
		KubeVersion: "v1.31.2", ApiVersions: []string{"v1"}, ReportedAt: timestamppb.New(time.Now().Add(-3 * time.Hour)),
	}))
}
