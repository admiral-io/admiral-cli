package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/version"
)

func newVersionCmd(ver version.Version) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Example: `  # Print the version, commit, and build date
  admiral version`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), ver.String())
			return err
		},
	}
}
