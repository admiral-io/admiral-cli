package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	userv1 "go.admiral.io/sdk/proto/admiral/api/user/v1"
)

// authStatus is the machine-readable shape of `auth status`. The first
// group describes the stored credential and is read without touching the
// secret store or the network; the second is what the server said about it.
type authStatus struct {
	Authenticated bool     `json:"authenticated" yaml:"authenticated"`
	Method        string   `json:"method,omitempty" yaml:"method,omitempty"`   // api-key | session
	Storage       string   `json:"storage,omitempty" yaml:"storage,omitempty"` // environment | file | 1password
	Path          string   `json:"path,omitempty" yaml:"path,omitempty"`       // credentials file, when storage is file
	Ref           string   `json:"ref,omitempty" yaml:"ref,omitempty"`         // reference, when storage is external
	Server        string   `json:"server,omitempty" yaml:"server,omitempty"`
	Account       string   `json:"account,omitempty" yaml:"account,omitempty"` // email the session was issued to
	Issuer        string   `json:"issuer,omitempty" yaml:"issuer,omitempty"`
	Expires       string   `json:"expires,omitempty" yaml:"expires,omitempty"` // RFC 3339
	Scopes        []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`   // narrowed session only

	// Verified reports whether the server accepted the credential. False
	// with Authenticated true means the check was skipped (--no-verify) or
	// did not complete; Error says which.
	Verified bool      `json:"verified" yaml:"verified"`
	User     *identity `json:"user,omitempty" yaml:"user,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	expiry time.Time // source of Expires, for the human rendering
}

// identity is who the server says the credential belongs to.
type identity struct {
	Email       string `json:"email,omitempty" yaml:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	ID          string `json:"id" yaml:"id"`
}

// verifyIdentity asks the server who the active credential belongs to. It
// is the one place status resolves the credential (a 1Password read, a
// session refresh) and it is a variable so tests can answer for the server.
var verifyIdentity = func(ctx context.Context, opts *client.Options) (*userv1.User, error) {
	c, err := client.CreateClient(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer c.Close() //nolint:errcheck // best-effort cleanup

	resp, err := c.User().GetMe(ctx, &userv1.GetMeRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetUser(), nil
}

// expiresHuman renders the session expiry in the describe timestamp format
// with the time remaining, e.g. "Mon, 14 Sep 2026 16:04:33 -0400 (in 3m53s)".
func (s authStatus) expiresHuman() string {
	if s.expiry.IsZero() {
		return ""
	}
	when := s.expiry.Local().Format(output.DescribeTimeLayout)
	left := time.Until(s.expiry).Truncate(time.Second)
	if left <= 0 {
		return when + " (expired; refreshed on next use)"
	}
	return when + " (in " + left.String() + ")"
}

func newStatusCmd(opts *client.Options) *cobra.Command {
	var noVerify bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the active credential and who the server says it is",
		Long: `Show which credential the CLI will use, where it is stored, and the user
the server resolves it to. The server check is one request; --no-verify
skips it and reports the stored credential alone, without contacting
anything or opening a secret store.

Exit status is 4 when there is no usable credential or the server rejects
it, so a script can gate on it.`,
		Example: `  # Show the credential and confirm it against the server
  admiral auth status

  # Only what is stored; no network, no 1Password prompt
  admiral auth status --no-verify

  # Machine-readable, for scripts
  admiral auth status -o json`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st := localStatus(opts)

			var verifyErr error
			if st.Authenticated && !noVerify {
				user, err := verifyIdentity(cmd.Context(), opts)
				switch {
				case err == nil:
					st.Verified = true
					st.User = &identity{Email: user.GetEmail(), DisplayName: user.GetDisplayName(), ID: user.GetId()}
				case isAuthError(err):
					// The credential is there but the server will not take
					// it: a revoked key, a session whose refresh was refused.
					st.Authenticated = false
					st.Error = cmderr.Format(err)
					verifyErr = err
				default:
					st.Error = cmderr.Format(err)
					verifyErr = err
				}
			}

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
				switch {
				case st.Verified:
					d.Section("User", func(b *output.Block) {
						b.Field("Email", st.User.Email)
						b.Field("Display Name", st.User.DisplayName)
						b.Field("ID", st.User.ID)
					})
				case noVerify:
					d.Field("Verified", "no (--no-verify)")
				default:
					d.Field("Verified", "no ("+st.Error+")")
				}
			}
			if !st.Authenticated && !opts.OutputFormat.IsMachine() {
				d.Hint("Run 'admiral auth login' to sign in, or set " + credentials.EnvAPIKey + ".")
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			if err := p.PrintStatus(st, d); err != nil {
				return err
			}
			// The document is the answer for -o json|yaml; its fields carry
			// the outcome. In human mode the exit code does.
			if opts.OutputFormat.IsMachine() {
				return nil
			}
			if verifyErr != nil {
				return verifyErr
			}
			if !st.Authenticated {
				return cmderr.Auth("", errors.New("not signed in"))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "report the stored credential without contacting the server")
	return cmd
}

// storedIn renders the storage location for table output.
func storedIn(st authStatus) string {
	switch st.Storage {
	case "environment":
		return "environment (" + credentials.EnvAPIKey + ")"
	case "1password":
		return "1Password (" + st.Ref + ")"
	default:
		return output.Tildify(st.Path)
	}
}

// isAuthError reports whether err means the credential is not usable, as
// opposed to the server being unreachable.
func isAuthError(err error) bool {
	if errors.Is(err, credentials.ErrNotAuthenticated) || errors.Is(err, credentials.ErrSessionExpired) {
		return true
	}
	return status.Code(err) == codes.Unauthenticated
}

// localStatus describes the credential the CLI would present, from the
// environment and the credentials file alone. It never resolves a
// reference or refreshes a session: that is verification's job, and
// --no-verify must be able to report the credential without triggering
// either.
func localStatus(opts *client.Options) authStatus {
	st := authStatus{Server: opts.ServerAddr}

	if os.Getenv(credentials.EnvAPIKey) != "" {
		st.Authenticated, st.Method, st.Storage = true, "api-key", "environment"
		return st
	}

	cred, err := credentials.Load(opts.ConfigDir)
	if err != nil {
		var perr *credentials.ParseError
		switch {
		case errors.Is(err, fs.ErrNotExist):
			st.Error = credentials.ErrNotAuthenticated.Error()
		case errors.As(err, &perr):
			st.Error = fmt.Sprintf("%s is not valid (%v); run 'admiral auth login' to replace it", credentials.FilePath(opts.ConfigDir), perr.Err)
		default:
			st.Error = "reading credentials: " + err.Error()
		}
		return st
	}

	path := credentials.FilePath(opts.ConfigDir)
	switch cred.Kind {
	case credentials.KindAPIKey:
		if cred.APIKey == "" {
			st.Error = credentials.ErrNotAuthenticated.Error()
			return st
		}
		st.Authenticated, st.Method, st.Storage, st.Path = true, "api-key", "file", path
	case credentials.KindAPIKeyRef:
		if cred.Ref == "" {
			st.Error = credentials.ErrNotAuthenticated.Error()
			return st
		}
		st.Authenticated, st.Method, st.Storage, st.Ref = true, "api-key", "1password", cred.Ref
	case credentials.KindSession:
		if cred.AccessToken == "" {
			st.Error = credentials.ErrNotAuthenticated.Error()
			return st
		}
		st.Authenticated, st.Method, st.Storage, st.Path = true, "session", "file", path
		st.Issuer, st.Account, st.Scopes = cred.Issuer, cred.Email, cred.Scopes
		if !cred.Expiry.IsZero() {
			st.expiry = cred.Expiry
			st.Expires = cred.Expiry.Local().Format(time.RFC3339)
		}
	default:
		st.Error = fmt.Sprintf("credentials file has unknown kind %q; run 'admiral auth login' again", cred.Kind)
	}
	return st
}
