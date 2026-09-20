package input

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
)

// newCmd builds a minimal *cobra.Command with the given stdin and buffered
// stdout/stderr: never interactive, exactly like a script.
func newCmd(stdin io.Reader) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetIn(stdin)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func TestPromptLine_NotInteractiveIsUsageError(t *testing.T) {
	_, err := PromptLine(newCmd(strings.NewReader("value\n")), "API key", true)
	require.EqualError(t, err, "cannot prompt for API key when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestSecret_StdinFlagReadsAllOfAPipe(t *testing.T) {
	got, err := Secret(newCmd(strings.NewReader("-----BEGIN KEY-----\nline2\n-----END KEY-----\r\n")), "private key", true)
	require.NoError(t, err)
	require.Equal(t, "-----BEGIN KEY-----\nline2\n-----END KEY-----", got, "multi-line material is preserved; only the trailing CR/LF is trimmed")
}

// The flag was given but stdin is a terminal that must not be prompted (an
// agent is driving): the remedy is to pipe the value, not a flag that is
// already set.
func TestSecret_FlagOnANonInteractiveTerminal(t *testing.T) {
	t.Setenv("ADMIRAL_NO_INPUT", "1")
	cmd := newCmd(strings.NewReader(""))
	cmd.SetIn(os.Stdin) // a TTY when the test runs in one, a pipe otherwise
	if !iostreams.FromCommand(cmd).IsStdinTTY() {
		t.Skip("needs a terminal on stdin")
	}
	_, err := Secret(cmd, "API key", true)
	require.EqualError(t, err, "cannot prompt for API key when not running interactively; pipe it on stdin")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestSecret_WithoutFlagOnAPipeIsUsageError(t *testing.T) {
	_, err := Secret(newCmd(strings.NewReader("hunter2\n")), "password", false)
	require.EqualError(t, err, "--password-stdin required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestSecret_EmptyPipeIsEmpty(t *testing.T) {
	got, err := Secret(newCmd(strings.NewReader("")), "token", true)
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestReadRawLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"enter", "s3cret\r", "s3cret"},
		{"newline", "s3cret\n", "s3cret"},
		{"backspace edits", "pa\x7fss\r", "pss"},
		{"ctrl-u clears", "wrong\x15right\r", "right"},
		{"other control bytes ignored", "a\x1bb\r", "ab"},
		{"eof ends a non-empty line", "pasted", "pasted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readRawLine(strings.NewReader(tc.in))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Ctrl-C is an interrupt, not a failed read: the root prints "Interrupted."
// and exits 130 for context.Canceled.
func TestReadRawLine_CtrlCCancels(t *testing.T) {
	_, err := readRawLine(strings.NewReader("par\x03tial\r"))
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadRawLine_CtrlDOnEmptyIsEOF(t *testing.T) {
	_, err := readRawLine(strings.NewReader("\x04"))
	require.ErrorIs(t, err, io.EOF)

	got, err := readRawLine(strings.NewReader("ab\x04\r"))
	require.NoError(t, err)
	require.Equal(t, "ab", got, "Ctrl-D mid-line is ignored")
}
