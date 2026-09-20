// Package credentials resolves the credential a command presents to the
// Admiral API and manages the credentials file written by `admiral auth login`.
//
// Two kinds of credential exist:
//
//   - API key: a long-lived opaque key (admp_ for users, adms_ for agents).
//     Supplied via the ADMIRAL_API_KEY environment variable, or stored with
//     `admiral auth login --with-token`. Sent as "Authorization: Token <key>".
//     Instead of the key itself, the file may hold a reference such as
//     op://vault/item/field that is resolved on every use (see ref.go).
//
//   - Session: an OIDC access token obtained interactively by `admiral auth
//     login` (authorization code + PKCE), stored with its refresh token and
//     refreshed transparently. Sent as "Authorization: Bearer <jwt>".
//
// Resolution order: ADMIRAL_API_KEY, then the credentials file.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.admiral.io/sdk/client"
)

const (
	// EnvAPIKey supplies an API key directly, bypassing the credentials file.
	EnvAPIKey = "ADMIRAL_API_KEY"

	// credentialsFile is written by `admiral auth login`.
	credentialsFile = "credentials.json"

	// refreshWindow is how close to expiry a session is refreshed before use.
	refreshWindow = 30 * time.Second

	// refreshTimeout bounds a call to the token endpoint during refresh.
	refreshTimeout = 10 * time.Second
)

// Kind is the type of credential stored in the credentials file.
type Kind string

const (
	KindAPIKey    Kind = "api_key"
	KindAPIKeyRef Kind = "api_key_ref"
	KindSession   Kind = "session"
)

// Source identifies where a resolved credential came from.
type Source string

const (
	SourceEnv     Source = "env"     // ADMIRAL_API_KEY
	SourceAPIKey  Source = "api-key" // stored by `auth login --with-token`
	SourceSession Source = "session" // stored by `auth login`
)

// ErrNotAuthenticated is returned when no credential is available.
var ErrNotAuthenticated = errors.New("not signed in")

// ErrSessionExpired is returned when a stored session can no longer be used.
var ErrSessionExpired = errors.New("session expired")

// permissiveWarned remembers which paths have been warned about, so a
// command that loads the file more than once warns once.
var permissiveWarned sync.Map

// ParseError reports a credentials file that exists but is not valid JSON,
// typically the aftermath of a crash or a hand edit.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "parsing credentials: " + e.Err.Error() }

func (e *ParseError) Unwrap() error { return e.Err }

// TokenResult is a resolved credential ready to be attached to requests.
type TokenResult struct {
	Token      string
	AuthScheme client.AuthScheme
	Source     Source
}

// Credential is the on-disk shape of the credentials file. Exactly one kind
// is populated.
type Credential struct {
	Kind Kind `json:"kind"`

	// KindAPIKey
	APIKey string `json:"api_key,omitempty"`

	// KindAPIKeyRef: where to fetch the key, e.g. op://vault/item/field.
	Ref string `json:"ref,omitempty"`

	// KindSession
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`
	Issuer       string    `json:"issuer,omitempty"`    // identifies the provider; also the discovery fallback on logout
	ClientID     string    `json:"client_id,omitempty"` // public client id sent on refresh
	TokenURL     string    `json:"token_url,omitempty"` // avoids OIDC discovery on refresh

	// RevocationURL is the provider's RFC 7009 endpoint, discovered at login,
	// so logout revokes the refresh token without a second discovery round
	// trip. Empty for a session stored before this was recorded.
	RevocationURL string `json:"revocation_url,omitempty"`

	Email  string   `json:"email,omitempty"`  // shown by `auth status`
	Scopes []string `json:"scopes,omitempty"` // resource scopes the session was narrowed to
}

// ResolveToken returns the credential to use for API calls. The environment
// wins over the file. A stored session that is expired or within
// refreshWindow of expiry is refreshed and persisted before being returned.
func ResolveToken(configDir string) (*TokenResult, error) {
	if k := os.Getenv(EnvAPIKey); k != "" {
		return &TokenResult{Token: k, AuthScheme: client.AuthSchemeToken, Source: SourceEnv}, nil
	}

	cred, err := Load(configDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotAuthenticated
		}
		var perr *ParseError
		if errors.As(err, &perr) {
			return nil, fmt.Errorf("%s is not valid (%v); run 'admiral auth login' to replace it", FilePath(configDir), perr.Err)
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	switch cred.Kind {
	case KindAPIKey:
		if cred.APIKey == "" {
			return nil, ErrNotAuthenticated
		}
		return &TokenResult{Token: cred.APIKey, AuthScheme: client.AuthSchemeToken, Source: SourceAPIKey}, nil

	case KindAPIKeyRef:
		if cred.Ref == "" {
			return nil, ErrNotAuthenticated
		}
		key, err := ResolveRef(context.Background(), cred.Ref)
		if err != nil {
			return nil, fmt.Errorf("resolving stored credential reference: %w", err)
		}
		return &TokenResult{Token: key, AuthScheme: client.AuthSchemeToken, Source: SourceAPIKey}, nil

	case KindSession:
		if cred.AccessToken == "" {
			return nil, ErrNotAuthenticated
		}
		if cred.Expiry.IsZero() || time.Until(cred.Expiry) > refreshWindow {
			return &TokenResult{Token: cred.AccessToken, AuthScheme: client.AuthSchemeBearer, Source: SourceSession}, nil
		}
		refreshed, err := refreshSession(configDir, cred)
		if err != nil {
			slog.Debug("session refresh failed", "error", err)
			return nil, ErrSessionExpired
		}
		return &TokenResult{Token: refreshed.AccessToken, AuthScheme: client.AuthSchemeBearer, Source: SourceSession}, nil

	default:
		return nil, fmt.Errorf("credentials file has unknown kind %q; run 'admiral auth login' again", cred.Kind)
	}
}

// ForceRefresh refreshes the stored session regardless of its local expiry.
// Used after the server rejects a token that looked valid locally. Returns
// an error when the active credential is not a session.
func ForceRefresh(configDir string) (*TokenResult, error) {
	if os.Getenv(EnvAPIKey) != "" {
		return nil, errors.New("active credential is an API key; nothing to refresh")
	}

	cred, err := Load(configDir)
	if err != nil {
		return nil, ErrNotAuthenticated
	}
	if cred.Kind != KindSession {
		return nil, errors.New("active credential is an API key; nothing to refresh")
	}

	refreshed, err := refreshSession(configDir, cred)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	return &TokenResult{Token: refreshed.AccessToken, AuthScheme: client.AuthSchemeBearer, Source: SourceSession}, nil
}

// ProactiveRefresh refreshes a session that is still valid but will expire
// within window, so the next command starts with a fresh token. It is a no-op
// unless a session is the active credential and is inside the window.
func ProactiveRefresh(configDir string, window time.Duration) error {
	if os.Getenv(EnvAPIKey) != "" {
		return nil
	}

	cred, err := Load(configDir)
	if err != nil {
		return nil //nolint:nilerr // no credentials is not an error here
	}
	if cred.Kind != KindSession || cred.Expiry.IsZero() || cred.RefreshToken == "" || cred.TokenURL == "" {
		return nil
	}
	remaining := time.Until(cred.Expiry)
	if remaining > window || remaining <= 0 {
		return nil
	}

	_, err = refreshSession(configDir, cred)
	return err
}

// FilePath returns the location of the credentials file for configDir.
func FilePath(configDir string) string {
	return filepath.Join(configDir, credentialsFile)
}

// Save writes the credential to the credentials file, replacing any
// existing one. The write is atomic (temp file, fsync, rename) so a crash
// mid-write cannot leave a truncated file, and the result is always
// owner-only regardless of what mode an existing file had: os.WriteFile
// would keep a widened mode, and this file holds a refresh token.
func Save(configDir string, cred *Credential) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	tmp, err := os.CreateTemp(configDir, credentialsFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	tmpPath := tmp.Name()
	// On any failure below, remove the temp file so no half-written secret
	// is left behind.
	defer os.Remove(tmpPath) //nolint:errcheck // no-op after a successful rename

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(configDir, credentialsFile)); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	return nil
}

// Load reads the credentials file. Returns an error wrapping fs.ErrNotExist
// when nothing has been stored.
func Load(configDir string) (*Credential, error) {
	path := filepath.Join(configDir, credentialsFile)

	warnIfPermissive(path)

	data, err := os.ReadFile(path) //nolint:gosec // path is configDir + constant
	if err != nil {
		return nil, err
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, &ParseError{Err: err}
	}
	// Files written before the kind tag existed hold a session.
	if cred.Kind == "" && cred.AccessToken != "" {
		cred.Kind = KindSession
	}
	return &cred, nil
}

// Delete removes the credentials file. Missing file is not an error. The
// lock file beside it is deliberately left alone: unlinking it while another
// process holds the lock would let a third process create a fresh one that
// the holder does not exclude. It holds nothing and costs nothing.
func Delete(configDir string) error {
	err := os.Remove(filepath.Join(configDir, credentialsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// refreshSession exchanges the refresh token for a new access token and
// persists the result. It holds the credentials lock for the whole
// read-refresh-write and re-reads the file after acquiring it: when several
// commands start at once, the first to get the lock refreshes and the rest
// find its result on disk instead of presenting an already-rotated refresh
// token, which the server refuses.
func refreshSession(configDir string, cred *Credential) (*Credential, error) {
	var out *Credential
	err := withLock(configDir, func() error {
		current, err := Load(configDir)
		if err != nil {
			return err
		}
		// Someone else refreshed while we waited for the lock: their token
		// is the live one. A stale AccessToken on our side is the tell.
		if current.Kind == KindSession && current.AccessToken != cred.AccessToken {
			slog.Debug("session already refreshed by a concurrent invocation")
			out = current
			return nil
		}
		out, err = doRefresh(configDir, current)
		return err
	})
	return out, err
}

// doRefresh performs the token endpoint call and persists the result. The
// caller holds the credentials lock.
func doRefresh(configDir string, cred *Credential) (*Credential, error) {
	if cred.RefreshToken == "" || cred.TokenURL == "" {
		return nil, errors.New("session has no refresh token")
	}

	// Force the oauth2 library to actually refresh: it returns the existing
	// token untouched while more than its internal delta (10s) remains, and
	// treats a zero Expiry as never-expiring.
	tok := &oauth2.Token{
		AccessToken:  cred.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: cred.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	}

	// Public client: no secret, and client_id must travel in the form body.
	// Left to auto-detect, the oauth2 library tries HTTP Basic first, which
	// the auth server rejects for public clients.
	cfg := &oauth2.Config{
		ClientID: cred.ClientID,
		Endpoint: oauth2.Endpoint{TokenURL: cred.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	refreshed, err := cfg.TokenSource(ctx, tok).Token()
	if err != nil {
		return nil, err
	}

	next := *cred
	next.AccessToken = refreshed.AccessToken
	next.Expiry = refreshed.Expiry
	// The server rotates refresh tokens; keep the old one only if none came back.
	if refreshed.RefreshToken != "" {
		next.RefreshToken = refreshed.RefreshToken
	}

	if err := Save(configDir, &next); err != nil {
		return nil, err
	}
	return &next, nil
}

// warnIfPermissive warns once per process when the credentials file is
// readable by others, mirroring SSH's private key check. Best effort.
func warnIfPermissive(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	perm := info.Mode().Perm()
	if perm&fs.FileMode(0077) == 0 {
		return
	}
	if _, already := permissiveWarned.LoadOrStore(path, true); already {
		return
	}
	slog.Warn(fmt.Sprintf("%s is readable by other users (mode %04o); run: chmod 600 %s", path, perm, path))
}
