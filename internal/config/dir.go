package config

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the configuration directory for Admiral CLI.
// Resolution order:
//  1. $ADMIRAL_CONFIG_DIR (if set)
//  2. $XDG_CONFIG_HOME/admiral (if XDG set)
//  3. ~/.config/admiral (default)
func ConfigDir() (string, error) {
	if dir := os.Getenv("ADMIRAL_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "admiral"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".config", "admiral"), nil
}
