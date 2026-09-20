package input

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
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

// interactiveCmd returns a command whose session counts as interactive
// (ADMIRAL_FORCE_INTERACTIVE) with stdin scripted, so the prompt paths run
// under test without a terminal.
func interactiveCmd(t *testing.T, stdin string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	t.Setenv("ADMIRAL_FORCE_INTERACTIVE", "1")
	return pipedCmd(stdin)
}

func TestConfirm_Interactive(t *testing.T) {
	for _, tc := range []struct {
		answer string
		ok     bool
	}{
		{"y\n", true}, {"Y\n", true}, {"yes\n", true}, {" yes \n", true},
		{"n\n", false}, {"\n", false}, {"", false}, {"maybe\n", false},
	} {
		t.Run(strings.TrimSpace(tc.answer), func(t *testing.T) {
			cmd, errOut := interactiveCmd(t, tc.answer)
			err := Confirm(cmd, false, "Delete foo")
			require.Contains(t, errOut.String(), "Delete foo? [y/N] ")
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrCanceled)
			require.Contains(t, errOut.String(), "canceled")
		})
	}
}

func TestConfirmName_Interactive(t *testing.T) {
	cmd, errOut := interactiveCmd(t, "shop\n")
	require.NoError(t, ConfirmName(cmd, false, "Deleting shop is not reversible.", "shop"))
	require.Contains(t, errOut.String(), "Deleting shop is not reversible.\nType shop to confirm: ")

	cmd, errOut = interactiveCmd(t, "shpo\n")
	require.ErrorIs(t, ConfirmName(cmd, false, "", "shop"), ErrCanceled)
	require.Contains(t, errOut.String(), "canceled")
}

// Two prompts in one process share a reader: the second sees the line the
// first left buffered rather than losing it to a fresh bufio.Reader.
func TestPrompts_ShareOneReader(t *testing.T) {
	cmd, _ := interactiveCmd(t, "y\nshop\n")
	cmd.SetContext(iostreams.WithStreams(context.Background(), iostreams.FromCommand(cmd)))
	require.NoError(t, Confirm(cmd, false, "First"))
	require.NoError(t, ConfirmName(cmd, false, "", "shop"))
}

func TestRequireInteractiveOrForce(t *testing.T) {
	cmd, _ := pipedCmd("")
	err := RequireInteractiveOrForce(cmd, false)
	require.EqualError(t, err, "--force required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.NoError(t, RequireInteractiveOrForce(cmd, true))

	cmd, _ = interactiveCmd(t, "")
	require.NoError(t, RequireInteractiveOrForce(cmd, false))
}
