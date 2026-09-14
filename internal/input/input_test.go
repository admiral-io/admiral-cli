package input

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
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
