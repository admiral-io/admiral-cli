package credentials

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.admiral.io/sdk/client"
)

func session(expiry time.Time, refresh, tokenURL string) *Credential {
	return &Credential{
		Kind:         KindSession,
		AccessToken:  "jwt-1",
		RefreshToken: refresh,
		Expiry:       expiry,
		ClientID:     "admiral-cli",
		TokenURL:     tokenURL,
	}
}

func TestResolveToken_EnvAPIKey(t *testing.T) {
	t.Setenv(EnvAPIKey, "admp_env")

	got, err := ResolveToken(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "admp_env", got.Token)
	require.Equal(t, client.AuthSchemeToken, got.AuthScheme)
	require.Equal(t, SourceEnv, got.Source)
}

func TestResolveToken_EnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_stored"}))
	t.Setenv(EnvAPIKey, "admp_env")

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "admp_env", got.Token)
	require.Equal(t, SourceEnv, got.Source)
}

func TestResolveToken_StoredAPIKey(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_stored"}))

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "admp_stored", got.Token)
	require.Equal(t, client.AuthSchemeToken, got.AuthScheme)
	require.Equal(t, SourceAPIKey, got.Source)
}

func TestResolveToken_Session(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, session(time.Now().Add(time.Hour), "", "")))

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "jwt-1", got.Token)
	require.Equal(t, client.AuthSchemeBearer, got.AuthScheme)
	require.Equal(t, SourceSession, got.Source)
}

func TestResolveToken_SessionNoExpiry(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, session(time.Time{}, "", "")))

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "jwt-1", got.Token)
}

func TestResolveToken_NotAuthenticated(t *testing.T) {
	os.Unsetenv(EnvAPIKey)

	_, err := ResolveToken(context.Background(), t.TempDir())
	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestResolveToken_UnknownKind(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, &Credential{Kind: "mystery"}))

	_, err := ResolveToken(context.Background(), dir)
	require.ErrorContains(t, err, "unknown kind")
}

func TestLoad_LegacyFileWithoutKindIsSession(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialsFile),
		[]byte(`{"access_token":"jwt-legacy","refresh_token":"r"}`), 0600))

	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, KindSession, cred.Kind)
	require.Equal(t, "jwt-legacy", cred.AccessToken)
}

func TestResolveToken_ExpiredWithoutRefreshToken(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, Save(dir, session(time.Now().Add(-time.Minute), "", "")))

	_, err := ResolveToken(context.Background(), dir)
	require.ErrorIs(t, err, ErrSessionExpired)
}

// tokenServer is a minimal OAuth2 token endpoint that answers refresh_token
// grants and records what it received.
func tokenServer(t *testing.T, newRefresh string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Empty(t, r.Header.Get("Authorization"), "public client must not use HTTP Basic")
		seen = append(seen, r.Form.Get("grant_type")+":"+r.Form.Get("refresh_token")+":"+r.Form.Get("client_id"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "jwt-2",
			"token_type":    "Bearer",
			"refresh_token": newRefresh,
			"expires_in":    3600,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestResolveToken_RefreshesNearExpiry(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	srv, seen := tokenServer(t, "refresh-2")
	require.NoError(t, Save(dir, session(time.Now().Add(5*time.Second), "refresh-1", srv.URL)))

	got, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "jwt-2", got.Token)
	require.Equal(t, []string{"refresh_token:refresh-1:admiral-cli"}, *seen)

	// Rotated refresh token and new expiry were persisted; kind survives.
	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, KindSession, cred.Kind)
	require.Equal(t, "jwt-2", cred.AccessToken)
	require.Equal(t, "refresh-2", cred.RefreshToken)
	require.Equal(t, "admiral-cli", cred.ClientID)
	require.True(t, cred.Expiry.After(time.Now().Add(30*time.Minute)))
}

func TestRefresh_KeepsOldRefreshTokenWhenNoneReturned(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	srv, _ := tokenServer(t, "")
	require.NoError(t, Save(dir, session(time.Now().Add(-time.Minute), "refresh-1", srv.URL)))

	_, err := ResolveToken(context.Background(), dir)
	require.NoError(t, err)

	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "refresh-1", cred.RefreshToken)
}

func TestForceRefresh(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	srv, seen := tokenServer(t, "refresh-2")
	require.NoError(t, Save(dir, session(time.Now().Add(time.Hour), "refresh-1", srv.URL))) // looks valid locally

	got, err := ForceRefresh(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "jwt-2", got.Token)
	require.Len(t, *seen, 1)
}

func TestForceRefresh_APIKeyActive(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv(EnvAPIKey, "admp_env")
		_, err := ForceRefresh(context.Background(), t.TempDir())
		require.Error(t, err)
	})
	t.Run("stored", func(t *testing.T) {
		dir := t.TempDir()
		os.Unsetenv(EnvAPIKey)
		require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_stored"}))
		_, err := ForceRefresh(context.Background(), dir)
		require.Error(t, err)
	})
}

func TestSave_Permissions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_x"}))

	info, err := os.Stat(filepath.Join(dir, credentialsFile))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSave_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, session(time.Now().Add(time.Hour), "r", "u")))
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_x"}))

	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, KindAPIKey, cred.Kind)
	require.Empty(t, cred.AccessToken)
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Delete(dir)) // missing is fine

	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_x"}))
	require.NoError(t, Delete(dir))

	_, err := Load(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSave_RestoresOwnerOnlyMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, credentialsFile)
	require.NoError(t, os.WriteFile(path, []byte(`{"kind":"api_key","api_key":"old"}`), 0644))

	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_new"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm(), "a widened mode must not survive a rewrite")
}

func TestSave_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_x"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Equal(t, []string{credentialsFile}, names)
}

// rotatingTokenServer refuses any refresh token it has already seen, the
// way the real server does with reuse-refresh-tokens=false.
func rotatingTokenServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	var mu sync.Mutex
	used := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		atomic.AddInt32(&calls, 1)
		rt := r.Form.Get("refresh_token")
		mu.Lock()
		reused := used[rt]
		used[rt] = true
		mu.Unlock()
		if reused {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "jwt-" + rt,
			"token_type":    "Bearer",
			"refresh_token": rt + "x",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestResolveToken_ConcurrentRefreshHappensOnce(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	srv, calls := rotatingTokenServer(t)
	require.NoError(t, Save(dir, session(time.Now().Add(-time.Minute), "r1", srv.URL)))

	const n = 8
	var wg sync.WaitGroup
	results := make([]*TokenResult, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = ResolveToken(context.Background(), dir)
		}()
	}
	wg.Wait()

	for i := range n {
		require.NoError(t, errs[i], "invocation %d must not lose the refresh race", i)
		require.Equal(t, "jwt-r1", results[i].Token, "every invocation sees the one refreshed token")
	}
	require.EqualValues(t, 1, atomic.LoadInt32(calls), "exactly one token-endpoint call")

	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "r1x", cred.RefreshToken)
}

func TestForceRefresh_SkipsWhenAlreadyRefreshed(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	srv, calls := rotatingTokenServer(t)
	require.NoError(t, Save(dir, session(time.Now().Add(time.Hour), "r1", srv.URL)))

	// First 401 handler refreshes.
	first, err := ForceRefresh(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "jwt-r1", first.Token)

	// A second process that also got a 401 on the OLD token would reach
	// refreshSession with the stale credential; it must pick up the new one
	// rather than burn the rotated refresh token.
	stale := session(time.Now().Add(time.Hour), "r1", srv.URL) // AccessToken "jwt-1", now stale
	got, err := refreshSession(context.Background(), dir, stale)
	require.NoError(t, err)
	require.Equal(t, "jwt-r1", got.AccessToken)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
}

func TestResolveToken_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialsFile), []byte(`{"kind":"session","access_tok`), 0600))

	_, err := ResolveToken(context.Background(), dir)
	require.ErrorContains(t, err, "is not valid")
	require.ErrorContains(t, err, "admiral auth login")
	require.ErrorContains(t, err, credentialsFile)
}

func TestDelete_KeepsLockFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &Credential{Kind: KindAPIKey, APIKey: "admp_x"}))
	require.NoError(t, withLock(dir, func() error { return nil })) // creates the lock file

	require.NoError(t, Delete(dir))

	_, err := os.Stat(filepath.Join(dir, lockFileName))
	require.NoError(t, err, "lock file survives logout so concurrent holders stay excluded")
	_, err = Load(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// A caller's context reaches the token endpoint: a refresh that is still
// in flight when the context ends is abandoned and reported as the
// context's error, not as an expired session, so the root can say
// "Interrupted." instead of telling the user to sign in again.
func TestResolveToken_RefreshHonorsContext(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv(EnvAPIKey)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	require.NoError(t, Save(dir, session(time.Now().Add(-time.Minute), "r1", srv.URL)))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := ResolveToken(ctx, dir)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, ErrSessionExpired)
	require.Less(t, time.Since(start), refreshTimeout, "must not wait for the refresh budget")

	// The stored session is untouched: a later command retries the refresh.
	cred, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "r1", cred.RefreshToken)
}
