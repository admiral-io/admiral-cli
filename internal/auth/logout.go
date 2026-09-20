package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"go.admiral.io/cli/internal/credentials"
)

// revokeTimeout bounds the whole revocation attempt, including the discovery
// round trip a session stored before RevocationURL existed still needs.
const revokeTimeout = 10 * time.Second

// LogoutResult describes what logout removed. The credential is gone from
// disk in every case it reports; the fields say what it was.
type LogoutResult struct {
	// Kind is the credential that was removed. Empty when nothing was stored,
	// and also when the file was there but could not be read.
	Kind credentials.Kind

	// Removed reports whether a credentials file was deleted. Removed with an
	// empty Kind means the file was unreadable and was removed anyway.
	Removed bool
}

// Logout deletes the stored credential. For a browser session it also, best
// effort, revokes the refresh token at the identity provider so it cannot be
// replayed; a revocation that does not happen is reported as a warning, since
// the local credential is gone either way.
func Logout(ctx context.Context, configDir string) (LogoutResult, error) {
	cred, err := credentials.Load(configDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LogoutResult{}, nil
		}
		// Unreadable file: still remove it so the user ends up logged out.
		slog.Debug("could not read credentials before logout", "error", err)
	}

	if err := credentials.Delete(configDir); err != nil {
		return LogoutResult{}, fmt.Errorf("removing credentials: %w", err)
	}
	if cred == nil {
		return LogoutResult{Removed: true}, nil
	}

	if cred.Kind == credentials.KindSession && cred.RefreshToken != "" {
		ctx, cancel := context.WithTimeout(ctx, revokeTimeout)
		defer cancel()
		revokeSession(ctx, cred)
	}
	return LogoutResult{Kind: cred.Kind, Removed: true}, nil
}

// revokeSession asks the identity provider to revoke the session's refresh
// token. Failure is a warning, not an error: nothing can be done about it
// locally, but staying silent would let the user believe a token that is
// still usable elsewhere had been withdrawn.
func revokeSession(ctx context.Context, cred *credentials.Credential) {
	endpoint, err := revocationEndpoint(ctx, cred)
	if err == nil {
		if err = revoke(ctx, endpoint, cred.ClientID, cred.RefreshToken); err == nil {
			return
		}
		slog.Debug("refresh token revocation failed", "endpoint", endpoint, "error", err)
	} else {
		slog.Debug("no revocation endpoint for logout", "error", err)
	}

	where := cred.Issuer
	if where == "" {
		where = "the identity provider"
	}
	slog.Warn(fmt.Sprintf("logged out locally, but the session was not revoked at %s and stays valid until it expires", where),
		"error", err)
}

// revocationEndpoint returns where to send the revocation request. Sessions
// created by this version carry the endpoint discovered at login; older ones
// carry only the issuer, so its metadata is fetched here instead of assuming
// a path, which differs between providers.
func revocationEndpoint(ctx context.Context, cred *credentials.Credential) (string, error) {
	if cred.RevocationURL != "" {
		return cred.RevocationURL, nil
	}
	if cred.Issuer == "" {
		return "", errors.New("session carries neither a revocation endpoint nor an issuer")
	}

	provider, err := oidc.NewProvider(ctx, cred.Issuer)
	if err != nil {
		return "", fmt.Errorf("querying identity provider %q: %w", cred.Issuer, err)
	}
	var meta struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if err := provider.Claims(&meta); err != nil {
		return "", fmt.Errorf("reading provider metadata: %w", err)
	}
	if meta.RevocationEndpoint == "" {
		return "", fmt.Errorf("identity provider %q advertises no revocation endpoint", cred.Issuer)
	}
	return meta.RevocationEndpoint, nil
}

// revoke calls the RFC 7009 revocation endpoint as a public client.
func revoke(ctx context.Context, endpoint, clientID, token string) error {
	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", "refresh_token")
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revocation endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
