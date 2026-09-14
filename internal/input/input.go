package input

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
)

// IsTerminal reports whether fd refers to an interactive terminal.
func IsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

// ReaderIsTerminal reports whether r is backed by an interactive terminal.
// Anything without a file descriptor (a bytes.Buffer in tests, a pipe
// wrapper) is not.
func ReaderIsTerminal(r io.Reader) (fd uintptr, ok bool) {
	f, has := r.(interface{ Fd() uintptr })
	if !has {
		return 0, false
	}
	return f.Fd(), IsTerminal(f.Fd())
}

// PromptLine asks for a single line on stderr and reads it from stdin. When
// sensitive, the input is read without echo. It fails with a usage error
// when the session is not interactive so scripts never hang.
func PromptLine(cmd *cobra.Command, label string, sensitive bool) (string, error) {
	io := iostreams.FromCommand(cmd)
	if !io.Interactive() {
		return "", cmderr.Usage("cannot prompt for %s when not running interactively", label)
	}
	fmt.Fprintf(io.Err, "Enter %s: ", label)
	if sensitive {
		fd, _ := ReaderIsTerminal(io.In)
		b, err := term.ReadPassword(int(fd))
		fmt.Fprintln(io.Err)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		return requireNonEmpty(string(b))
	}
	scanner := bufio.NewScanner(io.In)
	if scanner.Scan() {
		return requireNonEmpty(scanner.Text())
	}
	return "", fmt.Errorf("no input provided")
}

// Secret obtains a secret value without ever taking it from a flag:
//
//   - --<label>-stdin with a pipe: read all of stdin (trailing CR/LF trimmed,
//     so multi-line material such as PEM keys stays intact);
//   - --<label>-stdin on a terminal, or no flag in an interactive session:
//     prompt with echo off;
//   - otherwise: a usage error naming --<label>-stdin.
func Secret(cmd *cobra.Command, label string, fromStdin bool) (string, error) {
	io := iostreams.FromCommand(cmd)
	if fromStdin && !io.IsStdinTTY() {
		b, err := readAll(io.In)
		if err != nil {
			return "", fmt.Errorf("read %s from stdin: %w", label, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if io.Interactive() {
		return PromptLine(cmd, label, true)
	}
	return "", cmderr.Usage("--%s-stdin required when not running interactively", label)
}

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func requireNonEmpty(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", fmt.Errorf("value cannot be empty")
	}
	return v, nil
}
