package util

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ExactArgs returns a PositionalArgs validator that requires exactly n arguments.
// When the count is wrong it prints the command's help followed by the error.
func ExactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			_ = cmd.Help()
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			return fmt.Errorf("requires %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}
