package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/credentials"
)

// fakeIdP is a minimal OIDC provider: discovery, JWKS, an authorize endpoint
// that immediately redirects back with a code, and a token endpoint that
// checks PKCE and returns a signed id_token carrying the nonce.
type fakeIdP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	clientID string
	code     string

	// captured from the client for assertions
	authorizeQuery url.Values
	tokenForm      url.Values
	tokenAuthHdr   string
	revokeForm     url.Values
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f := &fakeIdP{key: key, clientID: "admiral-cli", code: "test-code"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/oauth2/authorize",
			"token_endpoint":                        f.srv.URL + "/oauth2/token",
			"jwks_uri":                              f.srv.URL + "/oauth2/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"code_challenge_methods_supported":      []string{"S256"},
		})
	})
	mux.HandleFunc("/oauth2/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig",
		}}})
	})
	mux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		f.authorizeQuery = r.URL.Query()
		redirect := f.authorizeQuery.Get("redirect_uri")
		http.Redirect(w, r, redirect+"?code="+f.code+"&state="+url.QueryEscape(f.authorizeQuery.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		f.tokenForm = r.Form
		f.tokenAuthHdr = r.Header.Get("Authorization")

		// PKCE: S256(verifier) must equal the challenge sent to /authorize.
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.authorizeQuery.Get("code_challenge") {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}

		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		require.NoError(t, err)
		idToken, err := jwt.Signed(signer).Claims(map[string]any{
			"iss":   f.srv.URL,
			"sub":   "user-123",
			"aud":   f.clientID,
			"exp":   time.Now().Add(time.Hour).Unix(),
			"iat":   time.Now().Unix(),
			"nonce": f.authorizeQuery.Get("nonce"),
			"email": "martin@example.com",
		}).Serialize()
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-jwt",
			"token_type":    "Bearer",
			"refresh_token": "refresh-1",
			"expires_in":    900,
			"id_token":      idToken,
			"scope":         f.authorizeQuery.Get("scope"), // granted == requested
		})
	})
	mux.HandleFunc("/oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		f.revokeForm = r.Form
		w.WriteHeader(http.StatusOK)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// followRedirects acts as the browser: GET the authorize URL and follow the
// redirect to the CLI's loopback callback.
func followRedirects(t *testing.T) func(string) error {
	return func(u string) error {
		go func() {
			resp, err := http.Get(u) //nolint:gosec // test-only URL
			if err == nil {
				_ = resp.Body.Close()
			}
			require.NoError(t, err)
		}()
		return nil
	}
}

func TestLogin_EndToEnd(t *testing.T) {
	idp := newFakeIdP(t)
	dir := t.TempDir()

	var status strings.Builder
	res, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ClientID:    idp.clientID,
		ConfigDir:   dir,
		OpenBrowser: followRedirects(t),
		Status:      &status,
	})
	require.NoError(t, err)
	require.Equal(t, "martin@example.com", res.Email)
	require.Equal(t, "user-123", res.Subject)
	require.Contains(t, status.String(), "Your browser has been opened to visit:")
	require.Contains(t, status.String(), idp.authorizeQuery.Get("state"), "sign-in URL is always shown")

	// Authorize request shape: PKCE S256, loopback IP redirect, default scopes.
	q := idp.authorizeQuery
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("code_challenge"))
	require.NotEmpty(t, q.Get("nonce"))
	require.Equal(t, idp.clientID, q.Get("client_id"))
	require.True(t, strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:"), q.Get("redirect_uri"))
	require.True(t, strings.HasSuffix(q.Get("redirect_uri"), "/callback"))
	require.Equal(t, "openid offline_access", q.Get("scope"))
	require.Empty(t, res.Scopes, "no narrowing requested")

	// Token request: public client, client_id in the body, no Basic auth.
	require.Equal(t, "authorization_code", idp.tokenForm.Get("grant_type"))
	require.Equal(t, idp.clientID, idp.tokenForm.Get("client_id"))
	require.Equal(t, q.Get("redirect_uri"), idp.tokenForm.Get("redirect_uri"))
	require.Empty(t, idp.tokenAuthHdr)

	// Session persisted with everything refresh and logout need.
	sess, err := credentials.Load(dir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindSession, sess.Kind)
	require.Equal(t, "access-jwt", sess.AccessToken)
	require.Equal(t, "refresh-1", sess.RefreshToken)
	require.Equal(t, idp.srv.URL, sess.Issuer)
	require.Equal(t, idp.clientID, sess.ClientID)
	require.Equal(t, idp.srv.URL+"/oauth2/token", sess.TokenURL)
	require.Equal(t, "martin@example.com", sess.Email)
	require.WithinDuration(t, time.Now().Add(15*time.Minute), sess.Expiry, 10*time.Second)
	require.Empty(t, sess.Scopes)
}

func TestLogin_NarrowedScopes(t *testing.T) {
	idp := newFakeIdP(t)
	dir := t.TempDir()

	res, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   dir,
		Scopes:      []string{"app:read", "env:read"},
		OpenBrowser: followRedirects(t),
	})
	require.NoError(t, err)

	// Base scopes are kept and the resource scopes are appended, not swapped in.
	require.Equal(t, "openid offline_access app:read env:read", idp.authorizeQuery.Get("scope"))

	// Only resource scopes are reported and stored; identity scopes are noise.
	require.Equal(t, []string{"app:read", "env:read"}, res.Scopes)
	sess, err := credentials.Load(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"app:read", "env:read"}, sess.Scopes)
}

func TestLogin_BrowserUnavailablePrintsURL(t *testing.T) {
	idp := newFakeIdP(t)

	var status strings.Builder
	var captured string
	open := func(u string) error {
		captured = u
		// Simulate the user pasting the URL after the CLI printed it.
		followRedirects(t)(u) //nolint:errcheck
		return context.DeadlineExceeded
	}

	_, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   t.TempDir(),
		OpenBrowser: open,
		Status:      &status,
	})
	require.NoError(t, err)
	require.Contains(t, status.String(), "Could not open a browser")
	require.Contains(t, status.String(), captured)
}

func TestLogin_RejectsStateMismatch(t *testing.T) {
	idp := newFakeIdP(t)

	open := func(u string) error {
		parsed, err := url.Parse(u)
		require.NoError(t, err)
		q := parsed.Query()
		q.Set("state", "tampered")
		parsed.RawQuery = q.Encode()
		return followRedirects(t)(parsed.String())
	}

	_, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   t.TempDir(),
		OpenBrowser: open,
	})
	require.ErrorContains(t, err, "state mismatch")
}

func TestLogin_IdPErrorSurfaces(t *testing.T) {
	idp := newFakeIdP(t)

	open := func(u string) error {
		parsed, err := url.Parse(u)
		require.NoError(t, err)
		cb := parsed.Query().Get("redirect_uri") + "?error=access_denied&error_description=User+declined"
		return followRedirects(t)(cb)
	}

	_, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   t.TempDir(),
		OpenBrowser: open,
	})
	require.ErrorContains(t, err, "access_denied")
	require.ErrorContains(t, err, "User declined")
}

func TestLogin_Canceled(t *testing.T) {
	idp := newFakeIdP(t)
	ctx, cancel := context.WithCancel(context.Background())

	open := func(string) error {
		cancel() // user hits Ctrl+C before finishing in the browser
		return nil
	}

	_, err := Login(ctx, LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   t.TempDir(),
		OpenBrowser: open,
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestLogout_RevokesAndDeletes(t *testing.T) {
	idp := newFakeIdP(t)
	dir := t.TempDir()
	require.NoError(t, credentials.Save(dir, &credentials.Credential{
		Kind:         credentials.KindSession,
		AccessToken:  "jwt",
		RefreshToken: "refresh-1",
		Issuer:       idp.srv.URL,
		ClientID:     idp.clientID,
	}))

	kind, err := Logout(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindSession, kind)

	require.Equal(t, "refresh-1", idp.revokeForm.Get("token"))
	require.Equal(t, "refresh_token", idp.revokeForm.Get("token_type_hint"))
	require.Equal(t, idp.clientID, idp.revokeForm.Get("client_id"))

	_, err = credentials.Load(dir)
	require.Error(t, err)
}

func TestLogout_APIKeyDoesNotRevoke(t *testing.T) {
	idp := newFakeIdP(t)
	dir := t.TempDir()
	require.NoError(t, credentials.Save(dir, &credentials.Credential{Kind: credentials.KindAPIKey, APIKey: "admp_x"}))

	kind, err := Logout(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, credentials.KindAPIKey, kind)
	require.Nil(t, idp.revokeForm)

	_, err = credentials.Load(dir)
	require.Error(t, err)
}

func TestLogout_NothingStored(t *testing.T) {
	kind, err := Logout(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.Empty(t, kind)
}

func TestLogin_TimesOutWaitingForBrowser(t *testing.T) {
	prev := loginTimeout
	loginTimeout = 50 * time.Millisecond
	t.Cleanup(func() { loginTimeout = prev })

	idp := newFakeIdP(t)
	_, err := Login(context.Background(), LoginOptions{
		Issuer:      idp.srv.URL,
		ConfigDir:   t.TempDir(),
		OpenBrowser: func(string) error { return nil }, // user never completes
	})
	require.ErrorContains(t, err, "timed out")
}
