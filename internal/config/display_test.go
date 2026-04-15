package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayValue_SensitiveKeyMasked(t *testing.T) {
	// admp_-prefixed token: prefix preserved, rest replaced with asterisks of matching length.
	got := DisplayValue("token", "admp_bhMKmSDgBX4o8IBzDFlDszNg7kEIR7DZ2b-YpcjEB4I3iBZmt")
	require.Equal(t, "admp_"+repeat("*", len("bhMKmSDgBX4o8IBzDFlDszNg7kEIR7DZ2b-YpcjEB4I3iBZmt")), got)
}

func TestDisplayValue_SensitiveKeyNotSet(t *testing.T) {
	require.Equal(t, "(not set)", DisplayValue("token", ""))
}

func TestDisplayValue_NonSensitiveReturnsRaw(t *testing.T) {
	require.Equal(t, "localhost:8080", DisplayValue("server", "localhost:8080"))
}

func TestDisplayValue_FallsBackToDefault(t *testing.T) {
	require.Equal(t, "false", DisplayValue("insecure", ""))
	require.Equal(t, "table", DisplayValue("output", ""))
}

func TestDisplayValue_NoValueNoDefault(t *testing.T) {
	require.Equal(t, "(not set)", DisplayValue("server", ""))
}

func TestMaskSecret_NoPrefix(t *testing.T) {
	// Without an underscore-delimited prefix, the whole value is masked.
	require.Equal(t, "******", maskSecret("abcdef"))
}

func TestMaskSecret_TrailingUnderscore(t *testing.T) {
	// Underscore at the very end: no tail to mask, fall back to full-length mask.
	require.Equal(t, "*****", maskSecret("abcd_"))
}

func TestMaskSecret_Empty(t *testing.T) {
	require.Equal(t, "", maskSecret(""))
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}