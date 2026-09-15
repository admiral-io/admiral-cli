package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	// Save and clear all relevant env vars so tests are isolated.
	for _, key := range []string{"ADMIRAL_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		if orig, ok := os.LookupEnv(key); ok {
			t.Setenv(key, orig) // restored automatically after test
		}
		os.Unsetenv(key)
	}

	t.Run("ADMIRAL_CONFIG_DIR takes precedence", func(t *testing.T) {
		t.Setenv("ADMIRAL_CONFIG_DIR", "/custom/admiral")
		t.Setenv("XDG_CONFIG_HOME", "/should/be/ignored")

		got, err := ConfigDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/custom/admiral" {
			t.Fatalf("expected /custom/admiral, got %q", got)
		}
	})

	t.Run("XDG_CONFIG_HOME used when ADMIRAL_CONFIG_DIR unset", func(t *testing.T) {
		os.Unsetenv("ADMIRAL_CONFIG_DIR")
		t.Setenv("XDG_CONFIG_HOME", "/xdg/home")

		got, err := ConfigDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join("/xdg/home", "admiral")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("falls back to ~/.config/admiral", func(t *testing.T) {
		os.Unsetenv("ADMIRAL_CONFIG_DIR")
		os.Unsetenv("XDG_CONFIG_HOME")

		got, err := ConfigDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("unexpected error getting home dir: %v", err)
		}
		want := filepath.Join(homeDir, ".config", "admiral")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}
