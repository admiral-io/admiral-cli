package iostreams

import (
	"bytes"
	"strings"
	"testing"

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
