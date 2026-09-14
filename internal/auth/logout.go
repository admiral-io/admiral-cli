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

	"go.admiral.io/cli/internal/credentials"
)

// Logout deletes the stored credential. For a browser session it also, best
// effort, revokes the refresh token at the identity provider so it cannot be
// replayed. Returns the kind that was removed, or "" when nothing was stored.
func Logout(ctx context.Context, configDir string) (credentials.Kind, error) {
	cred, err := credentials.Load(configDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		// Unreadable file: still remove it so the user ends up logged out.
		slog.Debug("could not read credentials before logout", "error", err)
	}

	if err := credentials.Delete(configDir); err != nil {
		return "", fmt.Errorf("removing credentials: %w", err)
	}
	if cred == nil {
		return "", nil
	}

	if cred.Kind == credentials.KindSession && cred.RefreshToken != "" && cred.Issuer != "" {
		if err := revoke(ctx, cred.Issuer, cred.ClientID, cred.RefreshToken); err != nil {
			slog.Debug("refresh token revocation failed", "error", err)
		}
	}
	return cred.Kind, nil
}

// revoke calls the RFC 7009 revocation endpoint as a public client.
func revoke(ctx context.Context, issuer, clientID, token string) error {
	endpoint := strings.TrimRight(issuer, "/") + "/oauth2/revoke"

	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", "refresh_token")
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revocation endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
