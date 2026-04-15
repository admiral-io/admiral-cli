package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidKey(t *testing.T) {
	require.True(t, IsValidKey("server"))
	require.True(t, IsValidKey("token"))
	require.False(t, IsValidKey("bogus"))
}

func TestIsSensitive(t *testing.T) {
	require.True(t, IsSensitive("token"))
	require.False(t, IsSensitive("server"))
}

func TestIsBool(t *testing.T) {
	require.True(t, IsBool("insecure"))
	require.True(t, IsBool("plaintext"))
	require.False(t, IsBool("server"))
	require.False(t, IsBool("token"))
}