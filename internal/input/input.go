package input

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// PromptLine reads a single line from cmd's stdin. On a TTY it first writes
// "Enter value: " to stderr; when sensitive, the input is read without echo.
// Piped (non-TTY) input is read as a single line. Empty values return an error.
func PromptLine(cmd *cobra.Command, sensitive bool) (string, error) {
	in := cmd.InOrStdin()
	f, ok := in.(interface{ Fd() uintptr })
	isTTY := ok && term.IsTerminal(int(f.Fd()))

	if isTTY {
		fmt.Fprint(cmd.ErrOrStderr(), "Enter value: ")
		if sensitive {
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return "", fmt.Errorf("failed to read input: %w", err)
			}
			return requireNonEmpty(string(b))
		}
	}

	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		return requireNonEmpty(scanner.Text())
	}
	return "", fmt.Errorf("no input provided")
}

// ResolveSecret returns a single-line secret value
//
//  1. If both --<label> and --<label>-stdin are set, error (ambiguous).
//  2. If --<label>-stdin is set, read all of stdin (trailing CR/LF trimmed).
//  3. If --<label> is set, return that value.
//  4. Otherwise, if stdin is a TTY, prompt for the value without echoing.
//     If stdin is not a TTY, return an error — refuse to hang a script.
//
// Prefer this over FromFlagOrStdin for user-supplied secrets (passwords,
// tokens) so humans get an interactive prompt and pipelines stay scriptable.
func ResolveSecret(cmd *cobra.Command, label, value string, stdinFlag bool) (string, error) {
	if stdinFlag && value != "" {
		return "", fmt.Errorf("--%s and --%s-stdin are mutually exclusive", label, label)
	}
	if stdinFlag {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read %s from stdin: %w", label, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if value != "" {
		return value, nil
	}

	in := cmd.InOrStdin()
	f, ok := in.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return "", fmt.Errorf("--%s or --%s-stdin is required (no TTY for interactive prompt)", label, label)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Enter %s: ", label)
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	return requireNonEmpty(string(b))
}

// FromFlagOrStdin returns value when stdinFlag is false, otherwise the full
// contents of os.Stdin with trailing CR/LF trimmed. Passing both a value and
// stdinFlag=true is rejected so callers can't silently prefer one source.
//
// Unlike PromptLine, this reads the entire stdin — multi-line secrets such as
// SSH private keys must remain intact.
func FromFlagOrStdin(label, value string, stdinFlag bool) (string, error) {
	if stdinFlag && value != "" {
		return "", fmt.Errorf("--%s and --%s-stdin are mutually exclusive", label, label)
	}
	if stdinFlag {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read %s from stdin: %w", label, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return value, nil
}

func requireNonEmpty(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", fmt.Errorf("value cannot be empty")
	}
	return v, nil
}
