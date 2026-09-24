package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	appcmd "go.admiral.io/cli/cmd/app"
	authcmd "go.admiral.io/cli/cmd/auth"
	changesetcmd "go.admiral.io/cli/cmd/changeset"
	componentcmd "go.admiral.io/cli/cmd/component"
	configcmd "go.admiral.io/cli/cmd/config"
	credentialcmd "go.admiral.io/cli/cmd/credential"
	envcmd "go.admiral.io/cli/cmd/env"
	internalauth "go.admiral.io/cli/internal/auth"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/iostreams"
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
	root, err := newRootCmd(version, exit)
	if err != nil {
		output.Writef(os.Stderr, "Error: %s\n", err)
		exit(cmderr.ExitError)
		return
	}
	root.Execute(args)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.cmd.SetArgs(args)

	// Ctrl+C cancels the command's context: in-flight RPCs are canceled
	// server-side and the login callback server shuts down, instead of the
	// process being killed mid-request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// A second Ctrl+C kills the process outright. NotifyContext keeps the
	// signals registered until stop runs, so without this a command that
	// ignores the context (a prompt reading stdin, a dial to a black-holed
	// host) could only be ended with kill -9.
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := cmd.cmd.ExecuteContext(ctx); err != nil {
		stderr := cmd.cmd.ErrOrStderr()
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			output.Writef(stderr, "\nInterrupted.\n")
			cmd.exit(cmderr.ExitInterrupted)
			return
		}
		if !cmderr.IsReported(err) {
			output.Writef(stderr, "Error: %s\n", formatError(err))
			if hint := errorHint(err); hint != "" {
				output.Writef(stderr, "%s\n", hint)
			}
		}
		cmd.exit(exitCode(err))
	}
}

func newRootCmd(ver version.Version, exit func(int)) (*rootCmd, error) {
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// One Streams for the process, carried on the context so every
			// prompt and printer below shares it: --no-input reaches them
			// without touching the environment, and two prompts read
			// through one buffered reader.
			streams := iostreams.FromCommand(cmd)
			if root.noInput {
				streams.DisableInput()
			}
			cmd.SetContext(iostreams.WithStreams(cmd.Context(), streams))

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
				// Loud, not debug: a corrupt config.json would otherwise
				// silently drop a `config set server …` the user relies on.
				output.Writef(cmd.ErrOrStderr(), "Warning: %v; using defaults for the settings it holds.\n", err)
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
						return cmderr.Usage("invalid %s %q: %v", envTimeout, v, err)
					}
					clientOpts.Timeout = d
				}
			}
			// TLS can be weakened from the file as well as by flag. A flag
			// is typed each time; a file entry is easy to forget, so say
			// so unless the target is the local machine anyway.
			if !cmd.Flags().Changed("insecure") {
				if v := settings.Get("insecure"); v == "true" {
					clientOpts.Insecure = true
					warnWeakTLS(cmd, "insecure", clientOpts.ServerAddr)
				}
			}
			if !cmd.Flags().Changed("plaintext") {
				if v := settings.Get("plaintext"); v == "true" {
					clientOpts.PlainText = true
					warnWeakTLS(cmd, "plaintext", clientOpts.ServerAddr)
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
	}
	flags.Group(cmd)
	cmd.SetVersionTemplate("{{.Version}}")

	// A bad or unknown flag is a usage error: exit 2, one line, and a hint
	// to --help rather than the full usage block.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return cmderr.UsageHint("Run '"+c.CommandPath()+" --help' for usage.", "%s", err.Error())
	})

	defaultConfigPath, err := config.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locating the config directory: %w", err)
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
		changesetcmd.NewChangeSetCmd(&clientOpts).Cmd,
		componentcmd.NewComponentCmd(&clientOpts).Cmd,
		credentialcmd.NewCredentialCmd(&clientOpts).Cmd,
		authcmd.NewAuthCmd(&clientOpts).Cmd,
		configcmd.NewConfigCmd(&clientOpts).Cmd,
		newCompletionCmd(),
		newVersionCmd(ver),
	)

	root.cmd = cmd
	root.clientOpts = &clientOpts

	return root, nil
}

// warnWeakTLS tells the user that config.json, not a flag, turned off TLS
// verification (insecure) or TLS itself (plaintext) for a server that is
// not on this machine.
func warnWeakTLS(cmd *cobra.Command, key, server string) {
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		host = server
	}
	if host == "localhost" || host == "" {
		return
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return
	}
	effect := "the certificate of " + server + " is not verified"
	if key == "plaintext" {
		effect = "the connection to " + server + " is not encrypted"
	}
	output.Writef(cmd.ErrOrStderr(), "Warning: config.json sets %s=true, so %s; run 'admiral config unset %s' if that is not intended.\n", key, effect, key)
}
