package config

import (
	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
)

// ConfigCmd is the parent command for CLI configuration.
type ConfigCmd struct {
	Cmd *cobra.Command
}

// NewConfigCmd creates the config command tree.
func NewConfigCmd(opts *admiralclient.Options) *ConfigCmd {
	root := &ConfigCmd{}

	cmd := &cobra.Command{
		Use:           "config",
		Short:         "Manage CLI configuration",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}

	cmd.AddCommand(
		newSetCmd(opts),
		newGetCmd(opts),
		newListCmd(opts),
		newUnsetCmd(opts),
	)

	root.Cmd = cmd
	return root
}
