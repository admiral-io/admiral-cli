package input

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

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
	ios := iostreams.FromCommand(cmd)
	if !ios.Interactive() {
		return "", cmderr.Usage("cannot prompt for %s when not running interactively", label)
	}
	fmt.Fprintf(ios.Err, "Enter %s: ", label)
	// Echo can only be turned off on a real terminal; a forced-interactive
	// test session reads its answer cooked like any other line.
	if fd, ok := ReaderIsTerminal(ios.In); ok && sensitive {
		line, err := readSecret(ios.In, int(fd))
		fmt.Fprintln(ios.Err)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		return requireNonEmpty(line)
	}
	line, err := readLine(ios)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	return requireNonEmpty(line)
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
	ios := iostreams.FromCommand(cmd)
	if fromStdin && !ios.IsStdinTTY() {
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", fmt.Errorf("read %s from stdin: %w", label, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if ios.Interactive() {
		return PromptLine(cmd, label, true)
	}
	if fromStdin {
		return "", cmderr.Usage("cannot prompt for %s when not running interactively; pipe it on stdin", label)
	}
	return "", cmderr.Usage("--%s-stdin required when not running interactively", label)
}

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
	esc       = 0x1b
	del       = 0x7f
)

// readRawLine assembles a line from a raw-mode terminal one byte at a
// time: Enter ends it, Backspace and Ctrl-U edit it, Ctrl-D on an empty
// line is end of input, and Ctrl-C cancels with context.Canceled so the
// root reports an interrupt (exit 130) rather than a failed read.
// Backspace removes a whole rune, so deleting an é never leaves half of
// it behind, and an escape sequence (an arrow key: ESC [ A) is swallowed
// rather than typed into the secret as "[A".
func readRawLine(in io.Reader) (string, error) {
	var line []byte
	var b [1]byte
	next := func() (byte, error) {
		_, err := in.Read(b[:])
		return b[0], err
	}
	for {
		c, err := next()
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}
		switch {
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
				_, size := utf8.DecodeLastRune(line)
				line = line[:len(line)-size]
			}
		case c == ctrlU:
			line = line[:0]
		case c == esc:
			if err := skipEscapeSequence(next); err != nil {
				return "", err
			}
		case c < 0x20:
			// other control bytes: ignored
		default:
			line = append(line, c)
		}
	}
}

// skipEscapeSequence consumes the rest of a terminal escape sequence whose
// ESC has just been read. A CSI sequence (ESC [ ... final) runs to its
// final byte in 0x40..0x7e; an SS3 sequence (ESC O x, the keypad and
// F1..F4) is one more byte; anything else is a lone ESC followed by an
// ordinary byte, which is dropped too.
// Input ending mid-sequence ends the sequence; the caller sees the EOF on
// its next read.
func skipEscapeSequence(next func() (byte, error)) error {
	c, err := next()
	if err != nil || c != '[' {
		if err == nil && c == 'O' {
			_, err = next()
		}
		return ignoringEOF(err)
	}
	for {
		c, err := next()
		if err != nil {
			return ignoringEOF(err)
		}
		if c >= 0x40 && c <= 0x7e {
			return nil
		}
	}
}

func ignoringEOF(err error) error {
	if err == io.EOF {
		return nil
	}
	return err
}
