package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

func TestSetAndGet(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Set(dir, "server", "localhost:8080"))

	s, err := LoadSettings(dir)
	require.NoError(t, err)
	require.Equal(t, "localhost:8080", s.Get("server"))
}

func TestSet_InvalidKey(t *testing.T) {
	err := Set(t.TempDir(), "bogus", "value")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown config key")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestSet_InvalidBool(t *testing.T) {
	err := Set(t.TempDir(), "insecure", "yes")
	require.EqualError(t, err, `invalid value "yes" for insecure: must be true or false`)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestUnset(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Set(dir, "server", "localhost:8080"))
	require.NoError(t, Unset(dir, "server"))

	s, err := LoadSettings(dir)
	require.NoError(t, err)
	require.Equal(t, "", s.Get("server"))
}

func TestUnset_InvalidKey(t *testing.T) {
	err := Unset(t.TempDir(), "bogus")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown config key")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestLoadSettings_NoFile(t *testing.T) {
	s, err := LoadSettings(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, s)
}

func TestLoadSettings_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, settingsFile), []byte("not json"), 0600))

	_, err := LoadSettings(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse config")
}

func TestSet_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Set(dir, "server", "localhost:8080"))

	info, err := os.Stat(filepath.Join(dir, settingsFile))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSet_MultipleKeys(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Set(dir, "server", "localhost:8080"))
	require.NoError(t, Set(dir, "insecure", "true"))
	require.NoError(t, Set(dir, "output", "json"))

	s, err := LoadSettings(dir)
	require.NoError(t, err)
	require.Equal(t, "localhost:8080", s.Get("server"))
	require.Equal(t, "true", s.Get("insecure"))
	require.Equal(t, "json", s.Get("output"))
}
