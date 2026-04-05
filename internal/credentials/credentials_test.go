package credentials

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/config"
)

func TestResolveToken_EnvVar(t *testing.T) {
	t.Setenv(EnvToken, "env-token-789")

	got, err := ResolveToken(t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "env-token-789", got.Token)
}

func TestResolveToken_EnvVarTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, config.Set(dir, "token", "config-token"))

	t.Setenv(EnvToken, "env-token")

	got, err := ResolveToken(dir)
	require.NoError(t, err)
	require.Equal(t, "env-token", got.Token)
}

func TestResolveToken_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvToken)

	require.NoError(t, config.Set(dir, "token", "config-token"))

	got, err := ResolveToken(dir)
	require.NoError(t, err)
	require.Equal(t, "config-token", got.Token)
}

func TestResolveToken_NoToken(t *testing.T) {
	os.Unsetenv(EnvToken)

	_, err := ResolveToken(t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no token configured")
}
