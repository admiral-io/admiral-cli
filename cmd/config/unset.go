package config

import (
	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
)

func newUnsetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a configuration value",
		Args:  cobra.ExactArgs(1),
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
