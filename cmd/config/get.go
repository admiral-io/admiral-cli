package config

import (
	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

func newGetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long: `Print one key's value from the config file, or its built-in default if the
key is not set. Flags and ADMIRAL_* environment variables are not consulted;
use 'admiral config list' to see the effective value and where it comes from.`,
		Example: `  # Print the stored server address
  admiral config get server`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if err := config.CheckKey(key); err != nil {
				return err
			}

			s, err := config.LoadSettings(opts.ConfigDir)
			if err != nil {
				return err
			}

			output.Writef(cmd.OutOrStdout(), "%s: %s\n", key, config.DisplayValue(key, s.Get(key)))
			return nil
		},
	}
}
