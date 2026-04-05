package config

import (
	"text/tabwriter"

	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
)

func newListCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := config.LoadSettings(opts.ConfigDir)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			for _, k := range config.DisplayKeys {
				output.Writef(w, "%s:\t%s\n", k, config.DisplayValue(k, s.Get(k)))
			}
			return w.Flush()
		},
	}
}
