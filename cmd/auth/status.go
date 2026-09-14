package auth

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
)

// status is the machine-readable shape of `auth status`.
type status struct {
	Authenticated bool     `json:"authenticated" yaml:"authenticated"`
	Method        string   `json:"method,omitempty" yaml:"method,omitempty"`   // api-key | session
	Storage       string   `json:"storage,omitempty" yaml:"storage,omitempty"` // environment | file | 1password
	Path          string   `json:"path,omitempty" yaml:"path,omitempty"`       // credentials file, when storage is file
	Ref           string   `json:"ref,omitempty" yaml:"ref,omitempty"`         // reference, when storage is external
	Server        string   `json:"server,omitempty" yaml:"server,omitempty"`
	Account       string   `json:"account,omitempty" yaml:"account,omitempty"`
	Issuer        string   `json:"issuer,omitempty" yaml:"issuer,omitempty"`
	Expires       string   `json:"expires,omitempty" yaml:"expires,omitempty"` // RFC 3339
	Scopes        []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`   // narrowed session only
	Error         string   `json:"error,omitempty" yaml:"error,omitempty"`

	expiry time.Time // source of Expires, for the human rendering
}

// expiresHuman renders the session expiry in the describe timestamp format
// with the time remaining, e.g. "Mon, 14 Sep 2026 16:04:33 -0400 (in 3m53s)".
func (s status) expiresHuman() string {
	if s.expiry.IsZero() {
		return ""
	}
	return s.expiry.Local().Format(output.DescribeTimeLayout) +
		" (in " + time.Until(s.expiry).Truncate(time.Second).String() + ")"
}

func newStatusCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active credential",
		Long: `Show which credential the CLI will use and where it is stored, without
contacting the API. Use 'admiral whoami' to verify it against the server.`,
		Example: `  # Show the credential the CLI will use
  admiral auth status

  # Machine-readable, for scripts
  admiral auth status -o json`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st := buildStatus(opts)

			d := output.NewDescribe()
			if !st.Authenticated {
				d.Field("Authenticated", "no")
				d.Field("Reason", st.Error)
			} else {
				d.Field("Authenticated", "yes")
				d.Field("Method", st.Method)
				d.Field("Stored In", storedIn(st))
				d.Field("Server", st.Server)
				if st.Method == "session" {
					d.Field("Account", st.Account)
					d.Field("Auth Server", st.Issuer)
					d.Field("Expires", st.expiresHuman())
					if len(st.Scopes) > 0 {
						d.Field("Scopes", strings.Join(st.Scopes, ", "))
					} else {
						d.Field("Scopes", "full access")
					}
				}
			}
			if !st.Authenticated && !opts.OutputFormat.IsMachine() {
				d.Hint("Run 'admiral auth login' to sign in, or set " + credentials.EnvAPIKey + ".")
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			if err := p.PrintStatus(st, d); err != nil {
				return err
			}
			if !st.Authenticated && !opts.OutputFormat.IsMachine() {
				return cmderr.Auth("", errors.New("not signed in"))
			}
			return nil
		},
	}
}

// storedIn renders the storage location for table output.
func storedIn(st status) string {
	switch st.Storage {
	case "environment":
		return "environment (" + credentials.EnvAPIKey + ")"
	case "1password":
		return "1Password (" + st.Ref + ")"
	default:
		return tildify(st.Path)
	}
}

// tildify shortens a path under $HOME to ~/... for display.
func tildify(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

func buildStatus(opts *client.Options) status {
	st := status{Server: opts.ServerAddr}

	cred, err := credentials.ResolveToken(opts.ConfigDir)
	if err != nil {
		st.Error = err.Error()
		if errors.Is(err, credentials.ErrSessionExpired) {
			st.Method = "session"
		}
		return st
	}

	st.Authenticated = true
	switch cred.Source {
	case credentials.SourceEnv:
		st.Method, st.Storage = "api-key", "environment"

	case credentials.SourceAPIKey:
		st.Method, st.Storage, st.Path = "api-key", "file", credentials.FilePath(opts.ConfigDir)
		if c, err := credentials.Load(opts.ConfigDir); err == nil && c.Kind == credentials.KindAPIKeyRef {
			st.Storage, st.Path, st.Ref = "1password", "", c.Ref
		}

	case credentials.SourceSession:
		st.Method, st.Storage, st.Path = "session", "file", credentials.FilePath(opts.ConfigDir)
		if sess, err := credentials.Load(opts.ConfigDir); err == nil {
			st.Issuer = sess.Issuer
			st.Account = sess.Email
			st.Scopes = sess.Scopes
			if !sess.Expiry.IsZero() {
				st.expiry = sess.Expiry
				st.Expires = sess.Expiry.Local().Format(time.RFC3339)
			}
		}
	}
	return st
}
