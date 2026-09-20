package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	internalauth "go.admiral.io/cli/internal/auth"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
)

func newLoginCmd(opts *client.Options) *cobra.Command {
	var (
		noBrowser bool
		withToken bool
		scopeFlag []string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Admiral",
		Long: `Log in to Admiral and store a credential for later commands.

By default the CLI opens the Admiral sign-in page in your browser, waits for
you to finish, and stores a session that is refreshed automatically until you
run 'admiral auth logout' or the refresh token expires.

With --with-token, the CLI instead reads an API key from stdin and stores it.
Use this on machines where a browser is impractical, or to pin a CI-style
credential for a user account. Create keys in the Admiral UI.

The input may also be a 1Password reference such as op://Vault/Item/field.
The CLI then stores only the reference and fetches the key through the
1Password CLI (op) each time it runs, so the secret never lands on disk.

A browser session normally carries your full access. Use --scope to narrow
it to specific resource scopes, for example on a shared machine; the server
enforces the narrowed set on every call. Scopes are fixed for the life of the
session, so run 'admiral auth login' again to change them.

Either way the credential lands in credentials.json inside the config
directory, readable only by you. If ADMIRAL_API_KEY is set, it takes
precedence over whatever is stored.`,
		Example: `  # Browser login
  admiral auth login

  # Do not launch a browser; just print the sign-in URL (e.g. over SSH)
  admiral auth login --no-browser

  # Read-only session
  admiral auth login --scope app:read,env:read,run:read

  # Store an API key; prompts without echo on a terminal
  admiral auth login --with-token

  # Store an API key from a pipe
  echo "$ADMIRAL_KEY" | admiral auth login --with-token

  # Store a 1Password reference instead of the key itself
  echo "op://Engineering/admiral/api-key" | admiral auth login --with-token`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if withToken && noBrowser {
				return cmderr.Usage("--with-token and --no-browser are mutually exclusive")
			}
			if withToken && len(scopeFlag) > 0 {
				return cmderr.Usage("--scope applies to browser sign-in; an API key's scopes are fixed when the key is created")
			}
			scopeList, err := validateScopes(scopeFlag)
			if err != nil {
				return err
			}

			if err := refuseIfEnvKeySet(); err != nil {
				return err
			}

			if withToken {
				return loginWithToken(cmd, opts)
			}

			lo := internalauth.LoginOptions{
				Issuer:    opts.Issuer,
				ClientID:  opts.ClientID,
				Scopes:    scopeList,
				ConfigDir: opts.ConfigDir,
				Status:    cmd.ErrOrStderr(),
			}
			if noBrowser {
				lo.OpenBrowser = func(string) error { return errNoBrowser }
			}

			res, err := internalauth.Login(cmd.Context(), lo)
			if err != nil {
				return err
			}

			who := res.Email
			if who == "" {
				who = res.Subject
			}
			output.Writef(cmd.OutOrStdout(), "Logged in as %s.\n", who)
			if len(res.Scopes) > 0 {
				output.Writef(cmd.OutOrStdout(), "Session limited to: %s\n", strings.Join(res.Scopes, ", "))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not launch a browser; the sign-in URL is always printed")
	cmd.Flags().StringArrayVar(&scopeFlag, "scope", nil, "limit the session to these resource scopes (repeatable or comma-separated); default is full access")
	_ = cmd.RegisterFlagCompletionFunc("scope", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return userScopes(), cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().BoolVar(&withToken, "with-token", false, "read an API key from stdin and store it, instead of signing in via browser")
	return cmd
}

var errNoBrowser = errors.New("browser launch disabled")

// loginWithToken reads a key or a reference to one from stdin (prompted
// without echo on a terminal), validates the key's format, and stores it as
// the active credential. A reference is resolved once now so problems with
// the store surface immediately, but only the reference is persisted.
func loginWithToken(cmd *cobra.Command, opts *client.Options) error {
	if isTerminal(cmd) {
		output.Writef(cmd.ErrOrStderr(), "Paste your Admiral API key or an op:// reference (input is hidden).\n")
	}
	in, err := input.Secret(cmd, "API key", true)
	if err != nil {
		return fmt.Errorf("reading API key: %w", err)
	}

	cred := &credentials.Credential{Kind: credentials.KindAPIKey, APIKey: in}
	key := in
	if credentials.IsRef(in) {
		key, err = credentials.ResolveRef(cmd.Context(), in)
		if err != nil {
			return err
		}
		cred = &credentials.Credential{Kind: credentials.KindAPIKeyRef, Ref: in}
	}

	if err := sdkclient.ValidateAuthToken(key); err != nil {
		return fmt.Errorf("invalid API key: %w (expected a key such as admp_...)", err)
	}

	if err := credentials.Save(opts.ConfigDir, cred); err != nil {
		return err
	}

	if cred.Kind == credentials.KindAPIKeyRef {
		output.Writef(cmd.OutOrStdout(), "API key reference stored: %s\n", cred.Ref)
	} else {
		output.Writeln(cmd.OutOrStdout(), "API key stored.")
	}
	return nil
}

// refuseIfEnvKeySet stops a login that could never take effect: the
// environment variable always wins over the credentials file, so storing
// anything while it is set only causes confusion later.
func refuseIfEnvKeySet() error {
	if os.Getenv(credentials.EnvAPIKey) == "" {
		return nil
	}
	return fmt.Errorf("the value of the %s environment variable is being used for authentication.\nTo have the CLI store a credential instead, first clear it from the environment", credentials.EnvAPIKey)
}

func isTerminal(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(interface{ Fd() uintptr })
	return ok && input.IsTerminal(f.Fd())
}
