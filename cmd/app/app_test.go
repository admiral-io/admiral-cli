package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
)

func newTestAppCmd(t *testing.T) *AppCmd {
	t.Helper()
	return NewAppCmd(&client.Options{})
}

func TestCreateCmd_RequiresOneArg(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"create"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "requires 1 arg(s)")
}

func TestListCmd_NoExtraArgs(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"list", "extra-arg"})

	err := root.Cmd.Execute()
	require.Error(t, err)
}

func TestGetCmd_NoArgAndNoContext(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"get"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "app name or --id is required")
}

func TestGetCmd_IDSkipsNameResolution(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"get", "--id", "some-uuid"})

	err := root.Cmd.Execute()
	// Should get past validation and fail at client creation, not "no app specified".
	require.Error(t, err)
	require.NotContains(t, err.Error(), "app name or --id")
}

func TestDeleteCmd_NoArgAndNoContext(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"delete"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "app name or --id is required")
}

func TestDeleteCmd_IDSkipsNameResolution(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"delete", "--id", "some-uuid"})

	// Without --confirm, should error about confirm, not about missing app name.
	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "--confirm")
}

func TestDeleteCmd_RequiresConfirm(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"delete", "billing-api"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "--confirm")
}

func TestUpdateCmd_NoArgAndNoContext(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"update"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "app name or --id is required")
}

func TestUpdateCmd_IDSkipsNameResolution(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"update", "--id", "some-uuid", "--description", "new"})

	// Should get past validation and fail at client creation, not "no app specified".
	err := root.Cmd.Execute()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "app name or --id")
}

func TestUpdateCmd_RequiresAtLeastOneField(t *testing.T) {
	root := newTestAppCmd(t)
	root.Cmd.SetArgs([]string{"update", "billing-api"})

	err := root.Cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "at least one of --name, --label, or --description")
}
