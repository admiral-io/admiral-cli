package input

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

// pipedCmd returns a command whose streams are buffers: never interactive.
func pipedCmd(stdin string) (*cobra.Command, *bytes.Buffer) {
	var errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	return cmd, &errOut
}

func TestConfirm_ForceSkipsPrompt(t *testing.T) {
	cmd, errOut := pipedCmd("")
	require.NoError(t, Confirm(cmd, true, "Delete foo"))
	require.Empty(t, errOut.String())
}

func TestConfirm_NotInteractiveIsUsageError(t *testing.T) {
	cmd, _ := pipedCmd("y\n")
	err := Confirm(cmd, false, "Delete foo")
	require.EqualError(t, err, "--force required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestConfirmName_ForceSkipsPrompt(t *testing.T) {
	cmd, _ := pipedCmd("")
	require.NoError(t, ConfirmName(cmd, true, "Deleting billing is not reversible.", "billing"))
}

func TestConfirmName_NotInteractiveIsUsageError(t *testing.T) {
	cmd, _ := pipedCmd("billing\n")
	err := ConfirmName(cmd, false, "", "billing")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestErrCanceledExitsOne(t *testing.T) {
	require.Equal(t, cmderr.ExitError, cmderr.Code(ErrCanceled))
	require.True(t, errors.Is(ErrCanceled, ErrCanceled))
}
