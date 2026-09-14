package credentials

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.admiral.io/cli/internal/input"
)

// A reference is a non-secret pointer to where an API key lives, in the
// form <scheme>://<provider-specific-path>. It is stored in place of the key
// and resolved on every use, so the secret never touches disk.
//
// Supported schemes:
//
//	op://<vault>/<item>/<field>   1Password, via the `op` CLI
const refScheme1Password = "op"

// resolveTimeout bounds a single reference lookup. 1Password may need to
// prompt for biometric unlock, so this is generous.
const resolveTimeout = 60 * time.Second

// runCommand executes an external program and returns its stdout. Tests
// replace it to avoid depending on installed tools.
//
// The program inherits this process's stdin, and when stderr is a terminal
// it is mirrored there as well as captured. A secret store that needs to
// prompt (1Password with the desktop-app integration off, or a lapsed
// `op signin`) can then actually ask; the capture still feeds the error
// message if it fails anyway.
var runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin

	var captured strings.Builder
	if input.IsTerminal(os.Stderr.Fd()) {
		cmd.Stderr = io.MultiWriter(&captured, os.Stderr)
	} else {
		cmd.Stderr = &captured
	}

	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(captured.String()); msg != "" {
			return nil, fmt.Errorf("%s: %s", name, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// IsRef reports whether s looks like a credential reference rather than a
// literal key.
func IsRef(s string) bool {
	scheme, _, ok := strings.Cut(s, "://")
	return ok && scheme != "" && !strings.ContainsAny(scheme, " /")
}

// ResolveRef fetches the secret a reference points to.
func ResolveRef(ctx context.Context, ref string) (string, error) {
	scheme, _, ok := strings.Cut(ref, "://")
	if !ok {
		return "", fmt.Errorf("%q is not a credential reference", ref)
	}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	switch scheme {
	case refScheme1Password:
		return resolve1Password(ctx, ref)
	default:
		return "", fmt.Errorf("unsupported credential reference scheme %q (supported: op://)", scheme)
	}
}

func resolve1Password(ctx context.Context, ref string) (string, error) {
	if _, err := exec.LookPath("op"); err != nil {
		return "", errors.New("1Password CLI (op) not found in PATH; install it from https://developer.1password.com/docs/cli/")
	}
	out, err := runCommand(ctx, "op", "read", "--no-newline", ref)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", ref, err)
	}
	secret := strings.TrimSpace(string(out))
	if secret == "" {
		return "", fmt.Errorf("reading %s: empty value", ref)
	}
	return secret, nil
}
