package config

import (
	"slices"
	"strings"

	"go.admiral.io/cli/internal/cmderr"
)

// ValidKeys lists all recognized configuration keys.
var ValidKeys = []string{"insecure", "output", "plaintext", "server"}

// DisplayKeys defines the display order for `config list`.
var DisplayKeys = []string{"server", "insecure", "plaintext", "output"}

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

// CheckKey returns a usage error (exit 2) naming the valid keys when key is
// not one of them.
func CheckKey(key string) error {
	if IsValidKey(key) {
		return nil
	}
	return cmderr.Usage("unknown config key %q (valid keys: %s)", key, strings.Join(ValidKeys, ", "))
}

// IsBool reports whether a key expects a boolean value.
func IsBool(key string) bool {
	return slices.Contains(BoolKeys, key)
}
