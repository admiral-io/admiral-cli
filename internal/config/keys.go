package config

import "slices"

// ValidKeys lists all recognised configuration keys.
var ValidKeys = []string{"insecure", "output", "plaintext", "server", "token"}

// DisplayKeys defines the display order for `config list`.
var DisplayKeys = []string{"server", "insecure", "plaintext", "token", "output"}

// SensitiveKeys are masked in display output.
var SensitiveKeys = []string{"token"}

// BoolKeys are keys that only accept "true" or "false".
var BoolKeys = []string{"insecure", "plaintext"}

// Defaults for keys that have a default value.
var Defaults = map[string]string{
	"insecure":  "false",
	"output":    "table",
	"plaintext": "false",
}

// IsValidKey reports whether key is a recognised config key.
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