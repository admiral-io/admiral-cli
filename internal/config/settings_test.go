package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestIsValidKey(t *testing.T) {
	require.True(t, IsValidKey("server"))
	require.True(t, IsValidKey("token"))
	require.False(t, IsValidKey("bogus"))
}

func TestIsSensitive(t *testing.T) {
	require.True(t, IsSensitive("token"))
	require.False(t, IsSensitive("server"))
}

func TestSet_MultipleKeys(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Set(dir, "server", "localhost:8080"))
	require.NoError(t, Set(dir, "insecure", "true"))
	require.NoError(t, Set(dir, "token", "admp_test"))

	s, err := LoadSettings(dir)
	require.NoError(t, err)
	require.Equal(t, "localhost:8080", s.Get("server"))
	require.Equal(t, "true", s.Get("insecure"))
	require.Equal(t, "admp_test", s.Get("token"))
}
