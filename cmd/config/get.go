package config

import (
	"fmt"

	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
)

func newGetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsValidKey(key) {
				return fmt.Errorf("unknown config key %q (valid keys: %v)", key, config.ValidKeys)
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
