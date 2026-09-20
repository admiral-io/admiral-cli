package input

import (
	"bufio"
	"context"
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
		line, err := readSecret(io.In, int(fd))
		fmt.Fprintln(io.Err)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		return requireNonEmpty(line)
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
//   - otherwise: a usage error. Without the flag it names --<label>-stdin;
//     with the flag but a terminal that must not be prompted (an agent
//     driving the CLI, ADMIRAL_NO_INPUT) it says to pipe the value.
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
	if fromStdin {
		return "", cmderr.Usage("cannot prompt for %s when not running interactively; pipe it on stdin", label)
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

// readSecret reads one line from a terminal without echo. It puts the
// terminal in raw mode itself instead of calling term.ReadPassword: in raw
// mode Ctrl-C is a byte, so the read ends and the terminal is restored on
// the way out. Under ReadPassword it is a signal, which the root turns into
// context cancellation while the read keeps blocking, and the second Ctrl-C
// that then kills the process leaves echo off.
func readSecret(in io.Reader, fd int) (string, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state) //nolint:errcheck // best effort; the shell resets on exit anyway
	return readRawLine(in)
}

// Control bytes readRawLine acts on. Everything else below 0x20 is ignored.
const (
	ctrlC     = 0x03
	ctrlD     = 0x04
	backspace = 0x08
	ctrlU     = 0x15
	del       = 0x7f
)

// readRawLine assembles a line from a raw-mode terminal one byte at a
// time: Enter ends it, Backspace and Ctrl-U edit it, Ctrl-D on an empty
// line is end of input, and Ctrl-C cancels with context.Canceled so the
// root reports an interrupt (exit 130) rather than a failed read.
func readRawLine(in io.Reader) (string, error) {
	var line []byte
	var b [1]byte
	for {
		if _, err := in.Read(b[:]); err != nil {
			if err == io.EOF && len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}
		switch c := b[0]; {
		case c == '\r' || c == '\n':
			return string(line), nil
		case c == ctrlC:
			return "", fmt.Errorf("prompt canceled: %w", context.Canceled)
		case c == ctrlD:
			if len(line) == 0 {
				return "", io.EOF
			}
		case c == backspace || c == del:
			if len(line) > 0 {
				line = line[:len(line)-1]
			}
		case c == ctrlU:
			line = line[:0]
		case c < 0x20:
			// other control bytes: ignored
		default:
			line = append(line, c)
		}
	}
}
