// Package iostreams answers the three questions every command needs to ask
// before writing anything: is this stream a terminal, is a person there to
// answer a prompt, and may we use color. The answers come from the command's
// own reader and writers plus the environment, so a test that swaps in a
// bytes.Buffer is treated exactly like a pipe in CI.
package iostreams

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// agentMarkers are environment variables that AI coding agents set when they
// drive a CLI. When one is present there is no person to answer a prompt.
// (Mirrors gh's internal/agents detection.)
var agentMarkers = []string{
	"AI_AGENT",
	"CLAUDECODE",
	"CODEX_SANDBOX",
	"COPILOT_CLI",
	"GEMINI_CLI",
	"CURSOR_AGENT",
}

// Streams bundles a command's stdin, stdout and stderr with the TTY,
// interactivity and color decisions derived from them.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	inTTY  bool
	outTTY bool
	errTTY bool

	// forcedWidth is set by ADMIRAL_FORCE_TTY; zero means "not forced".
	forcedWidth int

	noInput      bool
	colorEnabled bool
	getenv       func(string) string

	// lines is the one buffered reader every prompt in the process reads
	// through, so bytes one prompt buffered past its newline are seen by
	// the next instead of being lost with a throwaway bufio.Reader.
	lines *bufio.Reader
}

type ctxKey struct{}

// WithStreams returns a context carrying s, which FromCommand then returns
// for any command run under it. The root builds one Streams per process
// and installs it this way, so a flag such as --no-input reaches every
// prompt without touching the environment.
func WithStreams(ctx context.Context, s *Streams) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromCommand returns the Streams installed on cmd's context, or builds one
// from cmd's reader and writers. cobra returns os.Stdin/Stdout/Stderr
// unless a test has installed replacements, so the TTY checks are real in
// a terminal and false under test or in a pipe.
func FromCommand(cmd *cobra.Command) *Streams {
	if ctx := cmd.Context(); ctx != nil {
		if s, ok := ctx.Value(ctxKey{}).(*Streams); ok {
			return s
		}
	}
	return New(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), os.Getenv)
}

// System builds Streams for the process's own stdin, stdout and stderr.
func System() *Streams {
	return New(os.Stdin, os.Stdout, os.Stderr, os.Getenv)
}

// New builds Streams from explicit reader, writers and an environment
// lookup. getenv is a parameter so tests can pin the environment.
func New(in io.Reader, out, err io.Writer, getenv func(string) string) *Streams {
	s := &Streams{
		In:     in,
		Out:    out,
		Err:    err,
		inTTY:  isTerminal(in),
		outTTY: isTerminal(out),
		errTTY: isTerminal(err),
		getenv: getenv,
	}

	// ADMIRAL_FORCE_TTY=<cols> renders as if stdout and stderr were a
	// terminal of that width even when they are redirected. Tests use it to
	// exercise the human path; nothing in production sets it. (gh's
	// GH_FORCE_TTY.)
	if v := getenv("ADMIRAL_FORCE_TTY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.forcedWidth = n
		} else {
			s.forcedWidth = 80
		}
		s.outTTY = true
		s.errTTY = true
	}

	// ADMIRAL_FORCE_INTERACTIVE=1 is the same seam for prompts: the session
	// counts as interactive whatever stdin is (a buffer under test) and
	// whatever CI or agent markers the environment carries, and a prompt
	// reads its answer cooked. Kept apart from ADMIRAL_FORCE_TTY so forcing
	// output never invents a keyboard. Nothing in production sets it;
	// --no-input (DisableInput) still wins.
	if isSet(getenv("ADMIRAL_FORCE_INTERACTIVE")) {
		s.inTTY = true
		s.errTTY = true
	} else {
		s.noInput = nonInteractiveEnv(getenv)
	}

	s.colorEnabled = colorAllowed(getenv, s.outTTY)
	return s
}

// IsStdinTTY reports whether stdin is an interactive terminal.
func (s *Streams) IsStdinTTY() bool { return s.inTTY }

// IsStdoutTTY reports whether stdout is an interactive terminal.
func (s *Streams) IsStdoutTTY() bool { return s.outTTY }

// IsStderrTTY reports whether stderr is an interactive terminal.
func (s *Streams) IsStderrTTY() bool { return s.errTTY }

// DisableInput marks the session non-interactive, as ADMIRAL_NO_INPUT would;
// it is the --no-input flag's effect.
func (s *Streams) DisableInput() { s.noInput = true }

// LineReader returns the buffered reader prompts read their answers from.
// It wraps In once and is shared by every prompt on these Streams.
func (s *Streams) LineReader() *bufio.Reader {
	if s.lines == nil {
		s.lines = bufio.NewReader(s.In)
	}
	return s.lines
}

// Interactive reports whether a prompt may be shown: a person must be able
// to read it (stderr is a TTY), type the answer (stdin is a TTY), and not
// have asked us to stay quiet (ADMIRAL_NO_INPUT, CI, or an AI agent driving
// the CLI). Commands that need a value they cannot get without prompting must
// fail with an error naming the flag when this is false.
func (s *Streams) Interactive() bool {
	return s.inTTY && s.errTTY && !s.noInput
}

// ColorEnabled reports whether ANSI color may be written. The decision is
// made once for both stdout and stderr so a piped stdout never leaves a
// colored stderr behind. Precedence: NO_COLOR > CLICOLOR_FORCE > CLICOLOR=0
// > TERM=dumb > stdout is a TTY.
func (s *Streams) ColorEnabled() bool { return s.colorEnabled }

// TerminalWidth returns the width tables should fit within. It is the real
// terminal width when stdout is a TTY, the ADMIRAL_FORCE_TTY width when
// forced, and 80 otherwise.
func (s *Streams) TerminalWidth() int {
	if s.forcedWidth > 0 {
		return s.forcedWidth
	}
	if f, ok := s.Out.(interface{ Fd() uintptr }); ok && s.outTTY {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}
	if v := s.getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 80
}

func isTerminal(v any) bool {
	f, ok := v.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func nonInteractiveEnv(getenv func(string) string) bool {
	if isSet(getenv("ADMIRAL_NO_INPUT")) {
		return true
	}
	if isSet(getenv("CI")) {
		return true
	}
	for _, k := range agentMarkers {
		if getenv(k) != "" {
			return true
		}
	}
	return false
}

func colorAllowed(getenv func(string) string, stdoutTTY bool) bool {
	if getenv("NO_COLOR") != "" {
		return false
	}
	if getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	if getenv("CLICOLOR") == "0" {
		return false
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	return stdoutTTY
}

// isSet treats the common "off" spellings as unset so CI=false does what a
// person means.
func isSet(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}
