package config

import "slices"

// ValidKeys lists all recognized configuration keys.
var ValidKeys = []string{"insecure", "output", "plaintext", "server"}

// DisplayKeys defines the display order for `config list`.
var DisplayKeys = []string{"server", "insecure", "plaintext", "output"}

// SensitiveKeys are masked in display output. Credentials no longer live in
// config (see `admiral auth login`), so this is empty; DisplayValue keeps the
// masking path for any future secret key.
var SensitiveKeys = []string{}

// BoolKeys are keys that only accept "true" or "false".
var BoolKeys = []string{"insecure", "plaintext"}

// Defaults for keys that have a default value.
var Defaults = map[string]string{
	"server":    "api.admiral.io:443",
	"insecure":  "false",
	"output":    "table",
	"plaintext": "false",
}

// IsValidKey reports whether key is a recognized config key.
func IsValidKey(key string) bool {
	return slices.Contains(ValidKeys, key)
}

// IsSensitive reports whether a key should be masked in output.
func IsSensitive(key string) bool {
	return slices.Contains(SensitiveKeys, key)
}

// IsBool reports whether a key expects a boolean value.
func IsBool(key string) bool {
	return slices.Contains(BoolKeys, key)
}
