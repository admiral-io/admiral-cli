package config

import (
	"text/tabwriter"
	"unicode"

	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
)

func newListCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configuration values",
		Long:  `List all configuration values in the local CLI config file. Sensitive values (e.g. token) are masked.`,
		Example: `  # Show current configuration
  admiral config list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := config.LoadSettings(opts.ConfigDir)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			for _, k := range config.DisplayKeys {
				output.Writef(w, "%s:\t%s\n", title(k), config.DisplayValue(k, s.Get(k)))
			}
			return w.Flush()
		},
	}
}

func title(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
