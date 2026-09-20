package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildCLI compiles the admiral CLI into a temp dir and returns that dir, so
// tests exec the freshly built binary rather than whatever is on PATH.
func buildCLI() (string, error) {
	dir, err := os.MkdirTemp("", "admiral-e2e-bin-")
	if err != nil {
		return "", err
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "admiral"), ".")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %s", out)
	}
	return dir, nil
}

// scrubbedEnv is the process environment without any ADMIRAL_* variable,
// so a developer's own key or server never leaks into a scenario. Each
// scenario adds back exactly what it needs.
func scrubbedEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ADMIRAL_") {
			env = append(env, kv)
		}
	}
	return env
}

// serverStack names the live stack the server scenarios use, or skips the
// test when none is configured.
func serverStack(t *testing.T) (server, apiKey string) {
	t.Helper()
	server, apiKey = os.Getenv("ADMIRAL_SERVER"), os.Getenv("ADMIRAL_API_KEY")
	if server == "" || apiKey == "" {
		t.Skip("set ADMIRAL_SERVER and ADMIRAL_API_KEY to run against a live stack")
	}
	return server, apiKey
}
