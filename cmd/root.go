package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	appcmd "go.admiral.io/cli/cmd/app"
	configcmd "go.admiral.io/cli/cmd/config"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/version"
)

type rootCmd struct {
	cmd  *cobra.Command
	exit func(int)

	verbose      bool
	configPath   string
	outputFormat string
	clientOpts   *client.Options
}

func Execute(version version.Version, exit func(int), args []string) {
	newRootCmd(version, exit).Execute(args)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.cmd.SetArgs(args)

	if err := cmd.cmd.Execute(); err != nil {
		code := 1
		if eerr, ok := errors.AsType[*exitError](err); ok {
			code = eerr.code
		}
		output.Writef(os.Stderr, "Error: %s\n", formatError(err))
		cmd.exit(code)
	}
}

func newRootCmd(ver version.Version, exit func(int)) *rootCmd {
	var clientOpts client.Options

	root := &rootCmd{
		exit: exit,
	}

	cmd := &cobra.Command{
		Use:           "admiral",
		Short:         "Command-line client for the Admiral platform",
		Version:       ver.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if root.verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
				slog.Debug("debug logs enabled")
			}

			clientOpts.ConfigDir = root.configPath
			clientOpts.Verbose = root.verbose

			// Load persisted settings and apply as defaults for
			// any flags the user did not explicitly set.
			settings, err := config.LoadSettings(root.configPath)
			if err != nil {
				slog.Debug("failed to load config", "error", err)
			}

			if !cmd.Flags().Changed("server") {
				if v := settings.Get("server"); v != "" {
					clientOpts.ServerAddr = v
				}
			}
			if !cmd.Flags().Changed("insecure") {
				if v := settings.Get("insecure"); v == "true" {
					clientOpts.Insecure = true
				}
			}
			if !cmd.Flags().Changed("plaintext") {
				if v := settings.Get("plaintext"); v == "true" {
					clientOpts.PlainText = true
				}
			}
			if !cmd.Flags().Changed("output") {
				if v := settings.Get("output"); v != "" {
					root.outputFormat = v
				}
			}

			f, err := output.ParseFormat(root.outputFormat)
			if err != nil {
				return err
			}
			clientOpts.OutputFormat = f

			return nil
		},
	}
	cmd.SetVersionTemplate("{{.Version}}")

	defaultConfigPath, err := config.ConfigDir()
	if err != nil {
		slog.Error("failed to get default config path", "error", err)
		os.Exit(1)
	}

	// Config flags
	cmd.PersistentFlags().StringVar(&root.configPath, "config-dir", defaultConfigPath, "path to config directory")

	// Server flags
	cmd.PersistentFlags().StringVarP(&clientOpts.ServerAddr, "server", "s", "", "host:port of the API server")
	cmd.PersistentFlags().BoolVar(&clientOpts.PlainText, "plaintext", false, "disable TLS")
	cmd.PersistentFlags().BoolVarP(&clientOpts.Insecure, "insecure", "i", false, "skip server certificate and domain verification")

	// Output flags
	cmd.PersistentFlags().StringVarP(&root.outputFormat, "output", "o", "table", "output format: table, json, yaml, wide")

	// General flags
	cmd.PersistentFlags().BoolVarP(&root.verbose, "verbose", "v", false, "enable verbose mode")
	cmd.PersistentFlags().BoolP("help", "h", false, "help for admiral")

	// Resource commands
	cmd.AddCommand(
		appcmd.NewAppCmd(&clientOpts).Cmd,
	)

	// Configuration
	cmd.AddCommand(configcmd.NewConfigCmd(&clientOpts).Cmd)

	// Utility commands
	cmd.AddCommand(
		newCompletionCmd(),
		newVersionCmd(ver),
		newWhoamiCmd(&clientOpts),
	)

	root.cmd = cmd
	root.clientOpts = &clientOpts

	return root
}
