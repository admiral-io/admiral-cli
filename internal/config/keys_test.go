package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidKey(t *testing.T) {
	require.True(t, IsValidKey("server"))
	require.False(t, IsValidKey("api-key"), "credentials do not live in config")
	require.False(t, IsValidKey("bogus"))
}

func TestIsSensitive(t *testing.T) {
	require.False(t, IsSensitive("server"))
	require.False(t, IsSensitive("output"))
}

func TestIsBool(t *testing.T) {
	require.True(t, IsBool("insecure"))
	require.True(t, IsBool("plaintext"))
	require.False(t, IsBool("server"))
}
