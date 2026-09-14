package config

import (
	"fmt"
	"strings"

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
		Long: fmt.Sprintf(`Store a value in the config file. Omit the value to be prompted for it.

Valid keys: %s.
See 'admiral config --help' for what each key does.`,
			strings.Join(config.ValidKeys, ", "),
		),
		Example: `  # Point the CLI at a different server
  admiral config set server admiral.example.com:443

  # Make JSON the default output format
  admiral config set output json

  # Prompt for the value
  admiral config set server`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsValidKey(key) {
				return fmt.Errorf("unknown config key %q (valid keys: %s)", key, strings.Join(config.ValidKeys, ", "))
			}

			var value string
			if len(args) == 2 {
				value = args[1]
			} else {
				v, err := input.PromptLine(cmd, key, config.IsSensitive(key))
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
