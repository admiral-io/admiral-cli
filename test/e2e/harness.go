package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// buildCLI compiles the admiral CLI into a temp dir and returns that dir.
// Prepending it to PATH per-script means `exec admiral ...` resolves to
// the freshly built binary, not whatever is on the developer's machine.
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

// seedConfigDir creates a temp config dir and pre-populates it with the
// server address (and optional plaintext flag) so each script can share
// a single configured CLI without re-running `config set` every time.
// Token is *not* stored here — ADMIRAL_API_KEY env var overrides config,
// which keeps secrets out of the temp dir.
func seedConfigDir(admiralBin string) (string, error) {
	dir, err := os.MkdirTemp("", "admiral-e2e-config-")
	if err != nil {
		return "", err
	}
	run := func(args ...string) error {
		cmd := exec.Command(admiralBin, args...)
		cmd.Env = append(os.Environ(), "ADMIRAL_CONFIG_DIR="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("admiral %v: %s", args, out)
		}
		return nil
	}
	if err := run("config", "set", "server", os.Getenv("ADMIRAL_SERVER")); err != nil {
		return "", err
	}
	if os.Getenv("ADMIRAL_PLAINTEXT") != "" {
		if err := run("config", "set", "plaintext", "true"); err != nil {
			return "", err
		}
	}
	return dir, nil
}
