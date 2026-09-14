package config

import (
	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

func newUnsetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a configuration value",
		Long: `Remove a key from the config file so the built-in default applies again
(unless a flag or ADMIRAL_* environment variable overrides it).`,
		Example: `  # Go back to the default server
  admiral config unset server`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if err := config.Unset(opts.ConfigDir, key); err != nil {
				return err
			}
			output.Writef(cmd.OutOrStdout(), "%s: %s\n", key, config.DisplayValue(key, ""))
			return nil
		},
	}
}
