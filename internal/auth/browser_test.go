package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenBrowser_HonorsBrowserEnv(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened")
	script := "#!/bin/sh\necho \"$@\" > " + marker + "\n"
	bin := filepath.Join(dir, "fakebrowser")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0755)) //nolint:gosec // test stub

	t.Setenv("BROWSER", bin+" --private")
	require.NoError(t, openBrowser("https://example.test/login"))

	require.Eventually(t, func() bool {
		b, err := os.ReadFile(marker)
		return err == nil && string(b) == "--private https://example.test/login\n"
	}, 15*time.Second, 20*time.Millisecond) // generous: the whole suite may be running in parallel
}

func TestOpenBrowser_MissingBrowserEnvBinary(t *testing.T) {
	t.Setenv("BROWSER", filepath.Join(t.TempDir(), "nope"))
	require.Error(t, openBrowser("https://example.test/login"))
}
