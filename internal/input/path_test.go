package input

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	cases := map[string]string{
		"":              "",
		"~":             home,
		"~/":            home + "/",
		"~/x/y":         filepath.Join(home, "x", "y"),
		"~user/x":       "~user/x", // unsupported form is passed through
		"/abs/path":     "/abs/path",
		"relative/path": "relative/path",
		"a~b":           "a~b",
	}
	for in, want := range cases {
		got, err := ExpandPath(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
}
