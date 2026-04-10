package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

const settingsFile = "config.json"

// ValidKeys lists all recognised configuration keys.
var ValidKeys = []string{"insecure", "output", "plaintext", "server", "token"}

// DisplayKeys defines the display order for `config list`.
var DisplayKeys = []string{"server", "insecure", "plaintext", "token", "output"}

// SensitiveKeys are masked in display output.
var SensitiveKeys = []string{"token"}

// BoolKeys are keys that only accept "true" or "false".
var BoolKeys = []string{"insecure", "plaintext"}

// Settings holds persistent CLI configuration.
type Settings map[string]string

// LoadSettings reads the config file. Returns empty Settings if the file does
// not exist.
func LoadSettings(configDir string) (Settings, error) {
	path := filepath.Join(configDir, settingsFile)

	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from configDir + constant
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return s, nil
}

// IsBool reports whether a key expects a boolean value.
func IsBool(key string) bool {
	return slices.Contains(BoolKeys, key)
}

// Set persists a key-value pair.
func Set(configDir, key, value string) error {
	if !IsValidKey(key) {
		return fmt.Errorf("unknown config key %q (valid keys: %v)", key, ValidKeys)
	}
	if IsBool(key) && value != "true" && value != "false" {
		return fmt.Errorf("invalid value %q for %s: must be true or false", value, key)
	}

	s, err := LoadSettings(configDir)
	if err != nil {
		return err
	}

	s[key] = value
	return write(configDir, s)
}

// Unset removes a key from the config file.
func Unset(configDir, key string) error {
	if !IsValidKey(key) {
		return fmt.Errorf("unknown config key %q (valid keys: %v)", key, ValidKeys)
	}

	s, err := LoadSettings(configDir)
	if err != nil {
		return err
	}

	delete(s, key)
	return write(configDir, s)
}

// Get returns the value for a key, or "" if not set.
func (s Settings) Get(key string) string {
	return s[key]
}

// IsValidKey reports whether key is a recognised config key.
func IsValidKey(key string) bool {
	return slices.Contains(ValidKeys, key)
}

// IsSensitive reports whether a key should be masked in output.
func IsSensitive(key string) bool {
	return slices.Contains(SensitiveKeys, key)
}

// Defaults for keys that have a default value.
var Defaults = map[string]string{
	"insecure":  "false",
	"output":    "table",
	"plaintext": "false",
}

// DisplayValue returns the display string for a config key's raw value,
// handling masking of sensitive keys and falling back to defaults.
func DisplayValue(key, raw string) string {
	if IsSensitive(key) {
		if raw != "" {
			return "****"
		}
		return "(not set)"
	}
	if raw != "" {
		return raw
	}
	if d, ok := Defaults[key]; ok {
		return d
	}
	return "(not set)"
}

func write(configDir string, s Settings) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path := filepath.Join(configDir, settingsFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
