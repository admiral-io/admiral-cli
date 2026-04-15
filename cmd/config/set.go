package config

import (
	"fmt"

	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
)

func newSetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set a configuration value",
		Long: fmt.Sprintf(
			"Set a configuration value.\n\nValid keys: %v\n\nOmit the value to be prompted interactively. Sensitive keys (e.g. token) are read without echoing.",
			config.ValidKeys,
		),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsValidKey(key) {
				return fmt.Errorf("unknown config key %q (valid keys: %v)", key, config.ValidKeys)
			}

			var value string
			if len(args) == 2 {
				value = args[1]
			} else {
				v, err := input.PromptLine(cmd, config.IsSensitive(key))
				if err != nil {
					return err
				}
				value = v
			}

			if err := config.Set(opts.ConfigDir, key, value); err != nil {
				return err
			}

			output.Writef(cmd.OutOrStdout(), "%s: %s\n", key, config.DisplayValue(key, value))
			return nil
		},
	}
}

