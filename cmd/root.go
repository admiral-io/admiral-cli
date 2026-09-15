package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	appcmd "go.admiral.io/cli/cmd/app"
	authcmd "go.admiral.io/cli/cmd/auth"
	configcmd "go.admiral.io/cli/cmd/config"
	envcmd "go.admiral.io/cli/cmd/env"
	internalauth "go.admiral.io/cli/internal/auth"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/version"
)

const (
	// defaultServer is the Admiral service. Override with --server,
	// ADMIRAL_SERVER, or `config set server` for staging or local use.
	defaultServer = "api.admiral.io:443"

	envServer     = "ADMIRAL_SERVER"
	envAuthServer = "ADMIRAL_AUTH_SERVER"
	envClientID   = "ADMIRAL_CLIENT_ID"
	envTimeout    = "ADMIRAL_TIMEOUT"
)

type rootCmd struct {
	cmd  *cobra.Command
	exit func(int)

	verbose      bool
	noInput      bool
	configPath   string
	outputFormat string
	clientOpts   *client.Options
}

func Execute(version version.Version, exit func(int), args []string) {
	newRootCmd(version, exit).Execute(args)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.cmd.SetArgs(args)

	// Ctrl+C cancels the command's context: in-flight RPCs are canceled
	// server-side and the login callback server shuts down, instead of the
	// process being killed mid-request. A second Ctrl+C kills it outright.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			output.Writef(os.Stderr, "\nInterrupted.\n")
			cmd.exit(cmderr.ExitInterrupted)
			return
		}
		output.Writef(os.Stderr, "Error: %s\n", formatError(err))
		if hint := errorHint(err); hint != "" {
			output.Writef(os.Stderr, "%s\n", hint)
		}
		cmd.exit(exitCode(err))
	}
}

func newRootCmd(ver version.Version, exit func(int)) *rootCmd {
	var clientOpts client.Options

	root := &rootCmd{
		exit: exit,
	}

	cmd := &cobra.Command{
		Use:   "admiral",
		Short: "Command-line client for the Admiral platform",
		Long: `Command-line client for the Admiral platform.

Run 'admiral auth login' to get started, then 'admiral <command> --help'
for details on any command.

Documentation: https://admiral.io/docs`,
		Version:       ver.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// --no-input is the flag form of ADMIRAL_NO_INPUT; iostreams reads
			// the variable, so the flag just sets it for this process.
			if root.noInput {
				_ = os.Setenv("ADMIRAL_NO_INPUT", "1")
			}
			if root.verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
				slog.Debug("debug logs enabled")
			} else {
				slog.SetDefault(slog.New(output.NewLogHandler(cmd.ErrOrStderr(), slog.LevelWarn)))
			}

			clientOpts.ConfigDir = root.configPath
			clientOpts.Verbose = root.verbose

			// Precedence for the server, auth server, and client ID: flag,
			// then environment, then config, then the built-in production
			// default.
			settings, err := config.LoadSettings(root.configPath)
			if err != nil {
				slog.Debug("failed to load config", "error", err)
			}
			if settings.Get("token") != "" {
				output.Writef(cmd.ErrOrStderr(), "Warning: config.json contains a 'token' entry that is no longer read. Remove it; credentials belong in credentials.json via 'admiral auth login'.\n")
			}

			if !cmd.Flags().Changed("server") {
				if v := os.Getenv(envServer); v != "" {
					clientOpts.ServerAddr = v
				} else if v := settings.Get("server"); v != "" {
					clientOpts.ServerAddr = v
				}
			}
			if !cmd.Flags().Changed("auth-server") {
				if v := os.Getenv(envAuthServer); v != "" {
					clientOpts.Issuer = v
				}
			}
			if !cmd.Flags().Changed("client-id") {
				if v := os.Getenv(envClientID); v != "" {
					clientOpts.ClientID = v
				}
			}
			if !cmd.Flags().Changed("timeout") {
				if v := os.Getenv(envTimeout); v != "" {
					d, err := time.ParseDuration(v)
					if err != nil {
						return fmt.Errorf("invalid %s %q: %w", envTimeout, v, err)
					}
					clientOpts.Timeout = d
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
				return cmderr.Usage("%s", err.Error())
			}
			clientOpts.OutputFormat = f

			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Best-effort: if the login session is close to expiring,
			// refresh it now so the next command does not pay the latency.
			// Only after a command that actually used the API; there is no
			// point reading the credentials file after `config list`.
			if !client.Created() {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				if err := credentials.ProactiveRefresh(root.configPath, time.Minute); err != nil {
					slog.Debug("proactive session refresh failed", "error", err)
				}
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
				slog.Debug("proactive session refresh timed out")
			}
		},
	}
	cmd.SetVersionTemplate("{{.Version}}")

	// A bad or unknown flag is a usage error: exit 2, one line, and a hint
	// to --help rather than the full usage block.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return cmderr.UsageHint("Run '"+c.CommandPath()+" --help' for usage.", "%s", err.Error())
	})

	defaultConfigPath, err := config.ConfigDir()
	if err != nil {
		slog.Error("failed to get default config path", "error", err)
		os.Exit(1)
	}

	// Config flags
	cmd.PersistentFlags().StringVar(&root.configPath, "config-dir", defaultConfigPath, "path to config directory")

	// Server flags. Hidden: the CLI talks to the Admiral service by default,
	// and these exist for pointing it at staging or a local instance. The
	// ADMIRAL_SERVER environment variable does the same without a flag on
	// every command.
	cmd.PersistentFlags().StringVarP(&clientOpts.ServerAddr, "server", "s", defaultServer, "host:port of the API server")
	_ = cmd.PersistentFlags().MarkHidden("server")
	cmd.PersistentFlags().BoolVar(&clientOpts.PlainText, "plaintext", false, "connect without TLS (local development only)")
	_ = cmd.PersistentFlags().MarkHidden("plaintext")
	cmd.PersistentFlags().BoolVarP(&clientOpts.Insecure, "insecure", "i", false, "use TLS but do not verify the server certificate (self-signed staging hosts)")
	_ = cmd.PersistentFlags().MarkHidden("insecure")

	// Output flags
	cmd.PersistentFlags().StringVarP(&root.outputFormat, "output", "o", "table", "output format: table, wide, json, yaml, name")

	// Auth flags. Hidden: for pointing 'auth login' at a non-production
	// identity provider. The auth server is the OIDC issuer; its endpoints
	// are read from {auth-server}/.well-known/openid-configuration. The
	// ADMIRAL_AUTH_SERVER and ADMIRAL_CLIENT_ID environment variables do the
	// same without a flag.
	cmd.PersistentFlags().StringVar(&clientOpts.Issuer, "auth-server", internalauth.DefaultIssuer, "URL of the OIDC identity provider used by 'auth login'")
	_ = cmd.PersistentFlags().MarkHidden("auth-server")
	cmd.PersistentFlags().StringVar(&clientOpts.ClientID, "client-id", internalauth.DefaultClientID, "OAuth2 client ID used by 'auth login'")
	_ = cmd.PersistentFlags().MarkHidden("client-id")
	cmd.PersistentFlags().DurationVar(&clientOpts.Timeout, "timeout", client.DefaultTimeout, "per-request timeout (also ADMIRAL_TIMEOUT)")
	_ = cmd.PersistentFlags().MarkHidden("timeout")

	// General flags
	cmd.PersistentFlags().BoolVarP(&root.verbose, "verbose", "v", false, "enable verbose output")
	cmd.PersistentFlags().BoolVar(&root.noInput, "no-input", false, "never prompt; fail instead (also ADMIRAL_NO_INPUT=1)")

	cmd.AddCommand(
		appcmd.NewAppCmd(&clientOpts).Cmd,
		envcmd.NewEnvCmd(&clientOpts).Cmd,
		authcmd.NewAuthCmd(&clientOpts).Cmd,
		configcmd.NewConfigCmd(&clientOpts).Cmd,
		newWhoamiCmd(&clientOpts),
		newCompletionCmd(),
		newVersionCmd(ver),
	)

	root.cmd = cmd
	root.clientOpts = &clientOpts

	return root
}
