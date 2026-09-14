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

func TestScopeFlagsDefaultFromEnv(t *testing.T) {
	t.Setenv(EnvApp, "shop")
	t.Setenv(EnvEnv, "prod")
	var app, env string
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	App(cmd, &app, &client.Options{})
	Env(cmd, &env)
	_, err := exec(cmd)
	require.NoError(t, err)
	require.Equal(t, "shop", app)
	require.Equal(t, "prod", env)

	_, err = exec(cmd, "--app", "other")
	require.NoError(t, err)
	require.Equal(t, "other", app, "flag beats env")
}
