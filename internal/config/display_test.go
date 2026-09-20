package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayValue_ReturnsRaw(t *testing.T) {
	require.Equal(t, "localhost:8080", DisplayValue("server", "localhost:8080"))
}

func TestDisplayValue_FallsBackToDefault(t *testing.T) {
	require.Equal(t, "false", DisplayValue("insecure", ""))
	require.Equal(t, "table", DisplayValue("output", ""))
	require.Equal(t, "api.admiral.io:443", DisplayValue("server", ""))
}

func TestDisplayValue_NoValueNoDefault(t *testing.T) {
	require.Equal(t, "(not set)", DisplayValue("no-such-key", ""))
}
