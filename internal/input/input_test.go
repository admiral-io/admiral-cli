package input

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// newCmd builds a minimal *cobra.Command with the given stdin; stderr is
// captured into the returned buffer so prompt output can be asserted on.
func newCmd(stdin io.Reader) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetIn(stdin)
	cmd.SetErr(&stderr)
	return cmd, &stderr
}

// withStdin redirects os.Stdin to the given contents for the duration of fn,
// restoring the real stdin afterwards. Used to exercise the stdin-reading
// branches of FromFlagOrStdin / ResolveSecret.
func withStdin(t *testing.T, contents string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	go func() {
		defer w.Close()
		_, _ = io.WriteString(w, contents)
	}()

	fn()
}

func TestPromptLine_PipedSingleLine(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader("hello\n"))

	got, err := PromptLine(cmd, false)
	require.NoError(t, err)
	require.Equal(t, "hello", got)
}

func TestPromptLine_PipedTrimsWhitespace(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader("  spaced  \n"))

	got, err := PromptLine(cmd, false)
	require.NoError(t, err)
	require.Equal(t, "spaced", got)
}

func TestPromptLine_OnlyReadsFirstLine(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader("first\nsecond\n"))

	got, err := PromptLine(cmd, false)
	require.NoError(t, err)
	require.Equal(t, "first", got)
}

func TestPromptLine_EmptyInputErrors(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader(""))

	_, err := PromptLine(cmd, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no input")
}

func TestPromptLine_WhitespaceOnlyErrors(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader("   \n"))

	_, err := PromptLine(cmd, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "value cannot be empty")
}

func TestFromFlagOrStdin_ValueOnly(t *testing.T) {
	got, err := FromFlagOrStdin("password", "hunter2", false)
	require.NoError(t, err)
	require.Equal(t, "hunter2", got)
}

func TestFromFlagOrStdin_BothSetErrors(t *testing.T) {
	_, err := FromFlagOrStdin("password", "hunter2", true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestFromFlagOrStdin_EmptyReturnsEmpty(t *testing.T) {
	// No value, no stdin flag → empty string, no error. The caller decides
	// whether to treat that as "missing".
	got, err := FromFlagOrStdin("password", "", false)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestFromFlagOrStdin_StdinMultiLinePreserved(t *testing.T) {
	// SSH-key case: internal newlines must survive; only trailing CR/LF is trimmed.
	const key = "-----BEGIN KEY-----\nabcdef\n-----END KEY-----\n"
	withStdin(t, key, func() {
		got, err := FromFlagOrStdin("private-key", "", true)
		require.NoError(t, err)
		require.Equal(t, "-----BEGIN KEY-----\nabcdef\n-----END KEY-----", got)
	})
}

func TestFromFlagOrStdin_StdinTrimsCRLF(t *testing.T) {
	withStdin(t, "token-value\r\n", func() {
		got, err := FromFlagOrStdin("token", "", true)
		require.NoError(t, err)
		require.Equal(t, "token-value", got)
	})
}

func TestResolveSecret_ValueOnly(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader(""))

	got, err := ResolveSecret(cmd, "password", "s3cret", false)
	require.NoError(t, err)
	require.Equal(t, "s3cret", got)
}

func TestResolveSecret_BothSetErrors(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader(""))

	_, err := ResolveSecret(cmd, "password", "s3cret", true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestResolveSecret_StdinReadsFull(t *testing.T) {
	cmd, _ := newCmd(strings.NewReader(""))
	withStdin(t, "tok\n", func() {
		got, err := ResolveSecret(cmd, "token", "", true)
		require.NoError(t, err)
		require.Equal(t, "tok", got)
	})
}

func TestResolveSecret_NoTTYErrors(t *testing.T) {
	// Non-TTY stdin + neither flag: script-safe error, no hang.
	cmd, _ := newCmd(strings.NewReader(""))

	_, err := ResolveSecret(cmd, "token", "", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no TTY")
	require.Contains(t, err.Error(), "--token")
}