// Package auth implements the interactive browser login (OIDC authorization
// code flow with PKCE) and logout against the Admiral identity provider.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"go.admiral.io/cli/internal/credentials"
)

// DefaultIssuer is the Admiral identity provider used unless overridden with
// the hidden --auth-server flag (or ADMIRAL_AUTH_SERVER). It is the OIDC
// issuer: endpoints are discovered from its /.well-known/openid-configuration.
const DefaultIssuer = "https://auth.admiral.io"

// DefaultClientID is the public OAuth2 client registered for the CLI on the
// Admiral identity provider.
const DefaultClientID = "admiral-cli"

// contentSecurityPolicy allows exactly what the result page uses: its own
// markup plus an inline stylesheet. The logo and status glyphs are inline
// SVG, so the page needs no scripts, images, fonts or connections at all.
const contentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// DefaultScopes are requested at login. openid makes this an OIDC request
// with an id_token; offline_access makes the server issue a refresh token.
// No resource scopes: the API treats a user token that carries none as the
// user's full authority, and the identity provider puts email and name in
// the id_token regardless of scope, so "profile" is not needed either.
var DefaultScopes = []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess}

// loginTimeout bounds the whole browser round trip. Without it a login the
// user walked away from waits forever on the callback.
var loginTimeout = 5 * time.Minute // variable so tests can shorten it

//go:embed templates/result.gohtml
var resultPageRaw string

var resultPageTmpl = template.Must(template.New("result").Parse(resultPageRaw))

// successPage is rendered once at startup because it carries no per-request
// data and the callback handler should answer the browser immediately.
var successPage = renderResult(resultPage{
	Succeeded: true,
	Variant:   "success",
	Title:     "Authentication successful",
	Hint:      "You may now close this tab and return to the terminal.",
})

// LoginOptions configures the login flow.
type LoginOptions struct {
	Issuer    string
	ClientID  string
	ConfigDir string

	// Scopes are resource scopes (e.g. "app:read") that narrow the session
	// to a subset of the user's authority. They are added to DefaultScopes.
	// Empty means the session carries the user's full authority.
	Scopes []string

	// OpenBrowser launches the user's browser at the given URL. Tests
	// substitute this; nil uses the system browser.
	OpenBrowser func(url string) error

	// Status receives human-readable progress lines (e.g. the URL to open
	// when the browser cannot be launched). nil discards them.
	Status io.Writer
}

// Result describes a completed login.
type Result struct {
	Email   string
	Subject string
	Expiry  time.Time
	// Scopes are the resource scopes the server granted, if the session was
	// narrowed with LoginOptions.Scopes.
	Scopes []string
}

// resultPage is the data for templates/result.gohtml, the single page the
// loopback callback server ever serves.
type resultPage struct {
	Succeeded bool
	Variant   string // "success" or "error"; selects the badge's color and glyph
	Title     string
	Detail    string // provider-supplied text; rendered as escaped, quoted data
	Hint      string
}

// Login runs the authorization code + PKCE flow and persists the resulting
// session. It binds an ephemeral loopback port for the redirect, which the
// identity provider accepts for any port on 127.0.0.1 (RFC 8252 §7.3).
func Login(ctx context.Context, opts LoginOptions) (*Result, error) {
	if opts.Status == nil {
		opts.Status = io.Discard
	}
	if opts.OpenBrowser == nil {
		opts.OpenBrowser = openBrowser
	}
	if opts.ClientID == "" {
		opts.ClientID = DefaultClientID
	}

	// Loopback only, ephemeral port. The redirect must use the IP literal,
	// not "localhost": the auth server treats only IP literals as loopback
	// and refuses the hostname (RFC 8252 §8.3 says the same).
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("unable to open a loopback port for the login callback: %w", err)
	}
	defer ln.Close() //nolint:errcheck // best-effort cleanup

	redirectURL := fmt.Sprintf("http://%s/callback", ln.Addr().String())

	httpClient := &http.Client{Timeout: 30 * time.Second}
	oidcCtx := oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(oidcCtx, opts.Issuer)
	if err != nil {
		return nil, fmt.Errorf("querying identity provider %q: %w", opts.Issuer, err)
	}

	state, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}
	nonce, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	verifier := oauth2.GenerateVerifier()

	endpoint := provider.Endpoint()
	// Public client: client_id goes in the form body, never HTTP Basic.
	endpoint.AuthStyle = oauth2.AuthStyleInParams

	// The revocation endpoint is not part of oauth2.Endpoint, so read it from
	// the discovery document and store it with the session: logout then hits
	// the provider's real endpoint instead of guessing a path.
	var meta struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if err := provider.Claims(&meta); err != nil {
		slog.Debug("could not read provider metadata for the revocation endpoint", "error", err)
	}

	oc := &oauth2.Config{
		ClientID:    opts.ClientID,
		Endpoint:    endpoint,
		RedirectURL: redirectURL,
		Scopes:      append(append([]string{}, DefaultScopes...), opts.Scopes...),
	}

	authURL := oc.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	)

	type callback struct {
		code string
		err  error
	}
	resultCh := make(chan callback, 1)
	var once sync.Once
	deliver := func(c callback) { once.Do(func() { resultCh <- c }) }

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			renderError(w, http.StatusMethodNotAllowed, "Method not allowed.")
			return
		}
		q := r.URL.Query()

		if code := q.Get("error"); code != "" {
			desc := q.Get("error_description")
			renderError(w, http.StatusBadRequest, "Authorization failed: "+desc)
			deliver(callback{err: fmt.Errorf("authorization denied: %s: %s", code, desc)})
			return
		}
		if q.Get("state") != state {
			renderError(w, http.StatusBadRequest, "State mismatch. Please run the login command again.")
			deliver(callback{err: errors.New("state mismatch in login callback")})
			return
		}
		code := q.Get("code")
		if code == "" {
			renderError(w, http.StatusBadRequest, "Missing authorization code.")
			deliver(callback{err: errors.New("login callback carried no authorization code")})
			return
		}

		// Answer the browser immediately; the token exchange happens after.
		writeResultPage(w, http.StatusOK, successPage)
		deliver(callback{code: code})
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			deliver(callback{err: fmt.Errorf("login callback server failed: %w", err)})
		}
	}()
	defer shutdown(server)

	// Always show the URL. Browser launch can "succeed" and still land in
	// the wrong profile, a remote desktop, or nowhere at all.
	slog.Debug("opening browser for login", "url", authURL)
	if err := opts.OpenBrowser(authURL); err != nil {
		slog.Debug("browser launch failed", "error", err)
		fmt.Fprintf(opts.Status, "Could not open a browser. Visit this URL to sign in:\n\n    %s\n\n", authURL)
	} else {
		fmt.Fprintf(opts.Status, "Your browser has been opened to visit:\n\n    %s\n\n", authURL)
	}

	var cb callback
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("login canceled: %w", ctx.Err())
	case <-time.After(loginTimeout):
		return nil, fmt.Errorf("login timed out after %s waiting for the browser; run 'admiral auth login' again", loginTimeout)
	case cb = <-resultCh:
	}
	if cb.err != nil {
		return nil, cb.err
	}

	token, err := oc.Exchange(oidcCtx, cb.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("token response carried no id_token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: opts.ClientID}).Verify(oidcCtx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verifying id_token: %w", err)
	}
	if idToken.Nonce != nonce {
		return nil, errors.New("nonce mismatch in id_token")
	}

	var claims struct {
		Email string `json:"email"`
	}
	_ = idToken.Claims(&claims) // best effort; email is informational

	granted := resourceScopes(token)

	cred := &credentials.Credential{
		Kind:          credentials.KindSession,
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		Expiry:        token.Expiry,
		Issuer:        opts.Issuer,
		ClientID:      opts.ClientID,
		TokenURL:      endpoint.TokenURL,
		RevocationURL: meta.RevocationEndpoint,
		Email:         claims.Email,
		Scopes:        granted,
	}
	if err := Store(ctx, opts.ConfigDir, cred); err != nil {
		return nil, err
	}

	return &Result{Email: claims.Email, Subject: idToken.Subject, Expiry: token.Expiry, Scopes: granted}, nil
}

func renderResult(page resultPage) []byte {
	var buf bytes.Buffer
	if err := resultPageTmpl.Execute(&buf, page); err != nil {
		panic("rendering embedded auth result page: " + err.Error())
	}
	return buf.Bytes()
}

// writeResultPage serves an already-rendered page. The callback URL carries
// the authorization code in its query string, so the response must never be
// cached and must never leak the URL through a Referer header.
func writeResultPage(w http.ResponseWriter, status int, body []byte) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// resourceScopes extracts the resource scopes (those containing ":") the
// server granted, from the token response's space-separated scope field.
// Identity scopes are dropped; they are always present and never narrow.
func resourceScopes(token *oauth2.Token) []string {
	raw, _ := token.Extra("scope").(string)
	var out []string
	for _, s := range strings.Fields(raw) {
		if strings.Contains(s, ":") {
			out = append(out, s)
		}
	}
	return out
}

func renderError(w http.ResponseWriter, status int, msg string) {
	writeResultPage(w, status, renderResult(resultPage{
		Variant: "error",
		Title:   "Authentication failed",
		Detail:  msg,
		Hint:    "You may close this tab and return to the terminal.",
	}))
}

// shutdown drains in-flight responses so the browser receives the result
// page, then closes hard if that takes too long.
func shutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
	}
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
