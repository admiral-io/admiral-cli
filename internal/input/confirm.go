package input

import (
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
// is skipped when yes is true. When the session is not interactive
// (stdin or stderr is not a terminal, ADMIRAL_NO_INPUT or CI is set, or an
// AI agent is driving) it fails with a usage error instead of hanging.
//
// prompt is a question without the trailing "?": "Delete environment
// shop/staging". Declining returns ErrCanceled.
func Confirm(cmd *cobra.Command, yes bool, prompt string) error {
	if err := RequireInteractiveOrYes(cmd, yes); err != nil || yes {
		return err
	}
	ios := iostreams.FromCommand(cmd)
	fmt.Fprintf(ios.Err, "%s? [y/N] ", prompt)
	reply, err := readLine(ios)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(reply) {
	case "y", "yes":
		return nil
	default:
		fmt.Fprintln(ios.Err, "canceled")
		return ErrCanceled
	}
}

// ConfirmName is the severe-tier confirmation: it prints warning (a
// sentence such as "Deleting shop is not reversible and removes 2
// environments.") and requires the user to type name verbatim. Skipped when
// yes is true; a usage error when not interactive.
func ConfirmName(cmd *cobra.Command, yes bool, warning, name string) error {
	if err := RequireInteractiveOrYes(cmd, yes); err != nil || yes {
		return err
	}
	ios := iostreams.FromCommand(cmd)
	if warning != "" {
		fmt.Fprintln(ios.Err, warning)
	}
	fmt.Fprintf(ios.Err, "Type %s to confirm: ", name)
	reply, err := readLine(ios)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if reply != name {
		fmt.Fprintln(ios.Err, "canceled")
		return ErrCanceled
	}
	return nil
}

// RequireInteractiveOrYes is the check Confirm and ConfirmName make,
// exposed so a command can make it first: before signing in, dialing, or
// resolving names. A CI job that forgot --yes then fails in
// milliseconds with the usage error (exit 2) rather than after a round of
// RPCs, or with exit 4 because it was not signed in either.
func RequireInteractiveOrYes(cmd *cobra.Command, yes bool) error {
	if yes || iostreams.FromCommand(cmd).Interactive() {
		return nil
	}
	return cmderr.Usage("--yes required when not running interactively")
}

// readLine reads one answer through the Streams' shared reader, without
// its line ending or surrounding space.
func readLine(ios *iostreams.Streams) (string, error) {
	line, err := ios.LineReader().ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
