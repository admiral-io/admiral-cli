package credentials

import (
	"fmt"
	"log/slog"
	"os"

	"go.admiral.io/cli/internal/config"
)

const (
	EnvToken = "ADMIRAL_TOKEN"
)

type TokenResult struct {
	Token string
}

// ResolveToken returns a valid access token.
// Resolution order: ADMIRAL_TOKEN env var > config file.
func ResolveToken(configDir string) (*TokenResult, error) {
	// 1. Environment variable takes highest priority.
	if t := os.Getenv(EnvToken); t != "" {
		return &TokenResult{Token: t}, nil
	}

	// 2. Config file (set via `admiral config set token ...`).
	if t := configToken(configDir); t != "" {
		return &TokenResult{Token: t}, nil
	}

	return nil, fmt.Errorf("no token configured: set one with 'admiral config set token <PAT>' or export %s", EnvToken)
}

// configToken reads the token from the config file, returning "" on any error.
func configToken(configDir string) string {
	s, err := config.LoadSettings(configDir)
	if err != nil {
		slog.Debug("failed to load config for token", "error", err)
		return ""
	}
	return s.Get("token")
}
