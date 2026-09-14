package input

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
)

// ErrCanceled is returned when the user declines a confirmation.
var ErrCanceled = &cmderr.Error{Err: fmt.Errorf("canceled"), Code: cmderr.ExitError}

// Confirm asks a y/N question on stderr and reads the answer from stdin. It
// is skipped when force is true. When the session is not interactive
// (stdin or stderr is not a terminal, ADMIRAL_NO_INPUT or CI is set, or an
// AI agent is driving) it fails with a usage error instead of hanging.
//
// prompt is a question without the trailing "?": "Delete environment
// shop/staging". Declining returns ErrCanceled.
func Confirm(cmd *cobra.Command, force bool, prompt string) error {
	if force {
		return nil
	}
	io := iostreams.FromCommand(cmd)
	if !io.Interactive() {
		return notInteractive()
	}
	fmt.Fprintf(io.Err, "%s? [y/N] ", prompt)
	reply, err := readLine(io.In)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(reply)) {
	case "y", "yes":
		return nil
	default:
		fmt.Fprintln(io.Err, "canceled")
		return ErrCanceled
	}
}

// ConfirmName is the severe-tier confirmation: it prints warning (a
// sentence such as "Deleting shop is not reversible and removes 2
// environments.") and requires the user to type name verbatim. Skipped when
// force is true; a usage error when not interactive.
func ConfirmName(cmd *cobra.Command, force bool, warning, name string) error {
	if force {
		return nil
	}
	io := iostreams.FromCommand(cmd)
	if !io.Interactive() {
		return notInteractive()
	}
	if warning != "" {
		fmt.Fprintln(io.Err, warning)
	}
	fmt.Fprintf(io.Err, "Type %s to confirm: ", name)
	reply, err := readLine(io.In)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.TrimSpace(reply) != name {
		fmt.Fprintln(io.Err, "canceled")
		return ErrCanceled
	}
	return nil
}

// notInteractive is the error for a prompt that cannot be shown. It names the
// flag that answers the prompt.
func notInteractive() error {
	return cmderr.Usage("--force required when not running interactively")
}

func readLine(in io.Reader) (string, error) {
	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
