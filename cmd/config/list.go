package config

import (
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

// envOverrides maps config keys to the environment variables that override
// them, so the listing can say where a value actually came from.
var envOverrides = map[string]string{
	"server": "ADMIRAL_SERVER",
}

func newListCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configuration values and where each comes from",
		Long: `List every configuration key with its effective value and its origin:
a flag on this command, an environment variable, the config file, or the
built-in default. Credentials are not configuration; see 'admiral auth status'.`,
		Example: `  # Show current configuration
  admiral config list`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := config.LoadSettings(opts.ConfigDir)
			if err != nil {
				return err
			}

			path := filepath.Join(opts.ConfigDir, "config.json")
			rows := make([]settingRow, 0, len(config.DisplayKeys))
			for _, k := range config.DisplayKeys {
				value, source := resolve(cmd, k, s)
				rows = append(rows, settingRow{key: k, value: config.DisplayValue(k, value), source: source})
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if p.Format.IsMachine() {
				doc := make(map[string]settingJSON, len(rows))
				for _, r := range rows {
					doc[r.key] = settingJSON{Value: r.value, Source: r.source}
				}
				return p.PrintStatus(doc, nil)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			settingTable.Write(w, p, rows...)
			if err := w.Flush(); err != nil {
				return err
			}

			if _, err := os.Stat(path); err == nil {
				output.Writef(cmd.ErrOrStderr(), "\nConfig file: %s\n", output.Tildify(path))
			} else {
				output.Writef(cmd.ErrOrStderr(), "\nConfig file: %s (not created yet)\n", output.Tildify(path))
			}
			return nil
		},
	}
}

// resolve returns the effective value of key and a label for where it came
// from, mirroring the precedence the root command applies: flag, then
// environment, then config file, then default.
func resolve(cmd *cobra.Command, key string, s config.Settings) (value, source string) {
	if f := cmd.Flags().Lookup(key); f != nil && f.Changed {
		return f.Value.String(), "flag --" + key
	}
	if env, ok := envOverrides[key]; ok {
		if v := os.Getenv(env); v != "" {
			return v, "env " + env
		}
	}
	if v := s.Get(key); v != "" {
		return v, "config file"
	}
	return "", "default"
}

// settingRow is one resolved configuration value and where it came from.
type settingRow struct {
	key, value, source string
}

// settingJSON is the -o json shape of one setting.
type settingJSON struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

var settingTable = output.Table[settingRow]{
	{Header: "KEY", Cell: func(r settingRow) string { return r.key }},
	{Header: "VALUE", Cell: func(r settingRow) string { return r.value }},
	{Header: "SOURCE", Cell: func(r settingRow) string { return r.source }},
}
