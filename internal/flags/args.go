package flags

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
)

// ExactArgs validates that exactly n positional arguments were given. A
// mismatch is a usage error (exit 2) with a one-line message and a hint to
// the command's help; the full usage block is never dumped.
func ExactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == n {
			return nil
		}
		return usageError(cmd, n, len(args))
	}
}

// MaximumNArgs validates that at most n positional arguments were given.
func MaximumNArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) <= n {
			return nil
		}
		return cmderr.UsageHint(helpHint(cmd), "accepts at most %d arg(s), received %d", n, len(args))
	}
}

// RangeArgs validates that between least and most positional arguments
// were given.
func RangeArgs(least, most int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < least {
			return cmderr.UsageHint(helpHint(cmd), "missing argument: %s", cmd.Use)
		}
		if len(args) > most {
			return cmderr.UsageHint(helpHint(cmd), "accepts at most %d arg(s), received %d", most, len(args))
		}
		return nil
	}
}

// NoArgs validates that no positional arguments were given.
func NoArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return cmderr.UsageHint(helpHint(cmd), "unknown argument %q", args[0])
}

func usageError(cmd *cobra.Command, want, got int) error {
	if got < want {
		return cmderr.UsageHint(helpHint(cmd), "missing argument: %s", cmd.Use)
	}
	return cmderr.UsageHint(helpHint(cmd), "accepts %d arg(s), received %d", want, got)
}

func helpHint(cmd *cobra.Command) string {
	return "Run '" + cmd.CommandPath() + " --help' for usage."
}

// Group marks cmd as one that only groups subcommands. Cobra skips Args on
// a command that has no Run, so a parent with NoArgs still answers
// `admiral app bogus` with its help text and exit 0. Group gives the
// command a run that prints help and an Args that rejects an unknown
// subcommand as a usage error (exit 2), the same contract as a bad flag.
func Group(cmd *cobra.Command) {
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return cmderr.UsageHint(helpHint(cmd), "unknown command %q for %q", args[0], cmd.CommandPath())
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
}
