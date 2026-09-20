package flags

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
)

func exec(cmd *cobra.Command, args ...string) (string, error) {
	cmd.SilenceUsage, cmd.SilenceErrors = true, true // as the real root does
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestExactArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "get <name>", Args: ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
	_, err := exec(cmd, "x")
	require.NoError(t, err)

	printed, err := exec(cmd)
	require.EqualError(t, err, "missing argument: get <name>")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Equal(t, "Run 'get --help' for usage.", cmderr.Hint(err))
	require.Empty(t, printed, "no help dump")

	_, err = exec(cmd, "a", "b")
	require.EqualError(t, err, "accepts 1 arg(s), received 2")
}

func TestNoArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Args: NoArgs, RunE: func(*cobra.Command, []string) error { return nil }}
	_, err := exec(cmd, "extra")
	require.EqualError(t, err, `unknown argument "extra"`)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestEnum(t *testing.T) {
	var phase string
	cmd := &cobra.Command{Use: "logs", RunE: func(*cobra.Command, []string) error { return nil }}
	Enum(cmd, &phase, "phase", "", "phase to fetch", "plan", "apply")

	_, err := exec(cmd, "--phase", "apply")
	require.NoError(t, err)
	require.Equal(t, "apply", phase)

	_, err = exec(cmd, "--phase", "destroy")
	require.ErrorContains(t, err, "must be one of plan, apply")
	require.Contains(t, cmd.Flags().Lookup("phase").Usage, "plan, apply")
}

// Scope never comes from the shell: ADMIRAL_APP and ADMIRAL_ENV were
// removed (style guide §14 #25), so the flags have no default.
func TestScopeFlagsHaveNoDefault(t *testing.T) {
	t.Setenv("ADMIRAL_APP", "shop")
	t.Setenv("ADMIRAL_ENV", "prod")
	var app, env string
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	App(cmd, &app, &client.Options{})
	Env(cmd, &env, &client.Options{})
	_, err := exec(cmd)
	require.NoError(t, err)
	require.Empty(t, app)
	require.Empty(t, env)
	require.NotContains(t, cmd.Flags().Lookup("app").Usage, "ADMIRAL")
}

// appScoped runs cmd with a --app flag registered and returns what
// AppTarget makes of the positional arguments and the --app value.
func appScoped(t *testing.T, args []string, flagArgs ...string) (string, error) {
	t.Helper()
	var appFlag, got string
	var gotErr error
	cmd := &cobra.Command{Use: "x", RunE: func(cmd *cobra.Command, _ []string) error {
		got, gotErr = AppTarget(cmd, appFlag, args)
		return nil
	}}
	App(cmd, &appFlag, &client.Options{})
	_, err := exec(cmd, flagArgs...)
	require.NoError(t, err)
	return got, gotErr
}

func TestAppTarget_ArgumentNamesTheApp(t *testing.T) {
	app, err := appScoped(t, []string{"shop"})
	require.NoError(t, err)
	require.Equal(t, "shop", app)
}

func TestAppTarget_FallsBackToTheFlag(t *testing.T) {
	app, err := appScoped(t, nil, "--app", "shop")
	require.NoError(t, err)
	require.Equal(t, "shop", app)
}

// Neither given is not an error here: the command says what the missing
// scope means for itself.
func TestAppTarget_NeitherIsEmpty(t *testing.T) {
	app, err := appScoped(t, nil)
	require.NoError(t, err)
	require.Empty(t, app)
}

// Argument or flag, never both — even when they agree.
func TestAppTarget_ArgumentAndFlagIsUsageError(t *testing.T) {
	for _, flag := range []string{"shop", "other"} {
		_, err := appScoped(t, []string{"shop"}, "--app", flag)
		require.EqualError(t, err, "--app cannot be combined with an application argument")
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		require.Equal(t, "Name the application as the argument or with --app, not both.", cmderr.Hint(err))
	}
}

// A path addresses one environment, so it cannot scope a collection.
func TestAppTarget_PathIsUsageError(t *testing.T) {
	_, err := appScoped(t, []string{"shop/prod"})
	require.EqualError(t, err, `invalid application "shop/prod"`)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Contains(t, cmderr.Hint(err), "app/env addresses a single environment")
}

// scoped runs cmd with a --app flag registered and returns what EnvTarget
// makes of the given --app value and target.
func scoped(t *testing.T, target string, flagArgs ...string) (string, string, error) {
	t.Helper()
	var appFlag, gotApp, gotEnv string
	var gotErr error
	cmd := &cobra.Command{Use: "x", RunE: func(cmd *cobra.Command, _ []string) error {
		gotApp, gotEnv, gotErr = EnvTarget(cmd, appFlag, target)
		return nil
	}}
	App(cmd, &appFlag, &client.Options{})
	_, err := exec(cmd, flagArgs...)
	require.NoError(t, err)
	return gotApp, gotEnv, gotErr
}

func TestEnvTarget_BareNameTakesAppFromFlag(t *testing.T) {
	app, env, err := scoped(t, "prod", "--app", "shop")
	require.NoError(t, err)
	require.Equal(t, "shop", app)
	require.Equal(t, "prod", env)

	app, env, err = scoped(t, "prod")
	require.NoError(t, err)
	require.Empty(t, app, "no flag, no scope; resolve reports the missing app")
	require.Equal(t, "prod", env)
}

func TestEnvTarget_PathCarriesItsOwnScope(t *testing.T) {
	app, env, err := scoped(t, "shop/prod")
	require.NoError(t, err)
	require.Equal(t, "shop", app)
	require.Equal(t, "prod", env)
}

// Path or flag, never both — even when they agree.
func TestEnvTarget_PathAndFlagIsUsageError(t *testing.T) {
	for _, flag := range []string{"shop", "other"} {
		_, _, err := scoped(t, "shop/prod", "--app", flag)
		require.EqualError(t, err, "--app cannot be combined with a path")
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		require.Equal(t, "Give the environment as app/env or with --app, not both.", cmderr.Hint(err))
	}
}

func TestEnvTarget_MalformedPath(t *testing.T) {
	for _, target := range []string{"/prod", "shop/", "a/b/c"} {
		_, _, err := scoped(t, target)
		require.EqualError(t, err, `invalid environment "`+target+`": expected app/env`)
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	}
}

// A command without --app (changeset copy) can still take a path.
func TestEnvTarget_NoAppFlagRegistered(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	app, env, err := EnvTarget(cmd, "", "shop/prod")
	require.NoError(t, err)
	require.Equal(t, "shop", app)
	require.Equal(t, "prod", env)
}
