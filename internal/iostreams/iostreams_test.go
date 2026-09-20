package iostreams

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func env(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestBuffersAreNotTTY(t *testing.T) {
	s := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, env())
	require.False(t, s.IsStdinTTY())
	require.False(t, s.IsStdoutTTY())
	require.False(t, s.IsStderrTTY())
	require.False(t, s.Interactive())
	require.False(t, s.ColorEnabled())
	require.Equal(t, 80, s.TerminalWidth())
}

func TestForceTTY(t *testing.T) {
	s := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, env("ADMIRAL_FORCE_TTY", "120"))
	require.True(t, s.IsStdoutTTY())
	require.True(t, s.IsStderrTTY())
	require.False(t, s.IsStdinTTY(), "forcing output does not invent a keyboard")
	require.False(t, s.Interactive())
	require.True(t, s.ColorEnabled())
	require.Equal(t, 120, s.TerminalWidth())

	s = New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, env("ADMIRAL_FORCE_TTY", "yes"))
	require.Equal(t, 80, s.TerminalWidth(), "non-numeric value falls back to 80")
}

func TestNonInteractiveEnv(t *testing.T) {
	cases := map[string]func(string) string{
		"ADMIRAL_NO_INPUT": env("ADMIRAL_NO_INPUT", "1"),
		"CI":               env("CI", "true"),
		"CLAUDECODE":       env("CLAUDECODE", "1"),
		"AI_AGENT":         env("AI_AGENT", "x"),
	}
	for name, getenv := range cases {
		t.Run(name, func(t *testing.T) {
			require.True(t, nonInteractiveEnv(getenv))
		})
	}
	require.False(t, nonInteractiveEnv(env("CI", "false")), "CI=false is unset")
	require.False(t, nonInteractiveEnv(env("ADMIRAL_NO_INPUT", "0")))
	require.False(t, nonInteractiveEnv(env()))
}

func TestColorPrecedence(t *testing.T) {
	require.False(t, colorAllowed(env("NO_COLOR", "1", "CLICOLOR_FORCE", "1"), true), "NO_COLOR wins")
	require.True(t, colorAllowed(env("CLICOLOR_FORCE", "1"), false), "CLICOLOR_FORCE colors a pipe")
	require.False(t, colorAllowed(env("CLICOLOR", "0"), true))
	require.False(t, colorAllowed(env("TERM", "dumb"), true))
	require.True(t, colorAllowed(env(), true))
	require.False(t, colorAllowed(env(), false))
}

func TestForceInteractive(t *testing.T) {
	s := New(strings.NewReader("y\n"), &bytes.Buffer{}, &bytes.Buffer{}, env("ADMIRAL_FORCE_INTERACTIVE", "1"))
	require.True(t, s.IsStdinTTY())
	require.True(t, s.Interactive())
	require.False(t, s.IsStdoutTTY(), "forcing input does not force output")

	s.DisableInput()
	require.False(t, s.Interactive(), "--no-input wins over the forced keyboard")
}

func TestLineReaderIsShared(t *testing.T) {
	s := New(strings.NewReader("first\nsecond\n"), &bytes.Buffer{}, &bytes.Buffer{}, env())
	a, err := s.LineReader().ReadString('\n')
	require.NoError(t, err)
	b, err := s.LineReader().ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "first\n", a)
	require.Equal(t, "second\n", b, "the second prompt sees what the first left buffered")
}

func TestFromCommand_UsesInstalledStreams(t *testing.T) {
	installed := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, env())
	cmd := &cobra.Command{}
	cmd.SetContext(WithStreams(context.Background(), installed))
	require.Same(t, installed, FromCommand(cmd))

	require.NotSame(t, installed, FromCommand(&cobra.Command{}), "no context: built fresh")
}

func TestTerminalWidth_ColumnsViaGetenv(t *testing.T) {
	s := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, env("COLUMNS", "132"))
	require.Equal(t, 132, s.TerminalWidth())
}
