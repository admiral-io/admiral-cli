package config

import (
	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type ConfigCmd struct {
	Cmd *cobra.Command
}

func NewConfigCmd(opts *admiralclient.Options) *ConfigCmd {
	root := &ConfigCmd{}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
		Long: `Manage CLI settings stored in config.json inside the config directory.

Keys:
  server      host:port of the API server
  output      default output format: table, wide, json, yaml, name
  insecure    use TLS but do not verify the server certificate (true/false)
  plaintext   connect without TLS, for local development (true/false)

A stored value applies whenever the matching flag or ADMIRAL_* environment
variable is not given. Credentials are not configuration; see 'admiral auth'.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(
		newSetCmd(opts),
		newGetCmd(opts),
		newListCmd(opts),
		newUnsetCmd(opts),
	)

	root.Cmd = cmd
	return root
}
