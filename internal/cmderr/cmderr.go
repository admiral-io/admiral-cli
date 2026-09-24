// Package cmderr carries the two things the root command needs from an error
// beyond its message: the exit code, and an optional one-line hint naming
// the command or flag that fixes it.
package cmderr

import (
	"errors"
	"fmt"
)

// Exit codes are a contract with scripts: never renumber, and document any
// addition for users. (`admiral help exit-codes` is planned, not yet written.)
const (
	ExitOK          = 0
	ExitError       = 1   // runtime failure: server error, not found, failed run under --wait
	ExitUsage       = 2   // bad flag, missing positional, invalid enum value
	ExitAuth        = 4   // authentication required or expired
	ExitTimeout     = 124 // --wait-timeout elapsed
	ExitInterrupted = 130 // Ctrl-C
)

// Error is an error with an exit code and an optional hint line.
type Error struct {
	Err  error
	Code int
	Hint string
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// Hinter is implemented by errors that know how to fix themselves.
type Hinter interface {
	Hint() string
}

// Usage wraps a usage error (exit 2).
func Usage(format string, a ...any) error {
	return &Error{Err: fmt.Errorf(format, a...), Code: ExitUsage}
}

// UsageHint wraps a usage error (exit 2) with a hint line.
func UsageHint(hint, format string, a ...any) error {
	return &Error{Err: fmt.Errorf(format, a...), Code: ExitUsage, Hint: hint}
}

// Auth wraps an authentication error (exit 4).
func Auth(hint string, err error) error {
	return &Error{Err: err, Code: ExitAuth, Hint: hint}
}

// Timeout wraps a wait-timeout error (exit 124).
func Timeout(format string, a ...any) error {
	return &Error{Err: fmt.Errorf(format, a...), Code: ExitTimeout}
}

// WithHint attaches a hint to err without changing its exit code.
func WithHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return &Error{Err: e.Err, Code: e.Code, Hint: hint}
	}
	return &Error{Err: err, Code: ExitError, Hint: hint}
}

// Reported marks err as already shown to the user: the command's own output
// says what went wrong and how to fix it, so the root takes the exit code
// from err and prints nothing more.
func Reported(err error) error {
	if err == nil {
		return nil
	}
	return &reported{err: err}
}

type reported struct{ err error }

func (r *reported) Error() string { return r.err.Error() }

func (r *reported) Unwrap() error { return r.err }

// IsReported reports whether err was marked with Reported.
func IsReported(err error) bool {
	var r *reported
	return errors.As(err, &r)
}

// Code returns the exit code for err: the wrapped code when present,
// otherwise ExitError.
func Code(err error) int {
	var e *Error
	if errors.As(err, &e) && e.Code != 0 {
		return e.Code
	}
	return ExitError
}

// Hint returns the hint line for err, or "".
func Hint(err error) string {
	var e *Error
	if errors.As(err, &e) && e.Hint != "" {
		return e.Hint
	}
	var h Hinter
	if errors.As(err, &h) {
		return h.Hint()
	}
	return ""
}
