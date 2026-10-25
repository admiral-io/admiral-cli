package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/credentials"
)

func TestStateBaseURLIsTheAPIHostWithStateForAPI(t *testing.T) {
	for _, tc := range []struct {
		opts *Options
		want string
	}{
		{&Options{ServerAddr: "api.admiral.io:443"}, "https://state.admiral.io"},
		{&Options{ServerAddr: "api.dev.admiral.io"}, "https://state.dev.admiral.io"},
		{&Options{ServerAddr: "api.local:8080", PlainText: true}, "http://state.local"},
	} {
		got, err := StateBaseURL(tc.opts)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}

	_, err := StateBaseURL(&Options{ServerAddr: "localhost:8080"})
	assert.ErrorContains(t, err, EnvStateHost, "a host it cannot derive names the override")

	t.Setenv(EnvStateHost, "localhost:9090")
	got, err := StateBaseURL(&Options{ServerAddr: "localhost:8080", PlainText: true})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:9090", got)
}

func stateClientFor(t *testing.T, h http.HandlerFunc) *StateClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv(EnvStateHost, strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv(credentials.EnvAPIKey, "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG")
	c, err := NewStateClient(context.Background(), &Options{ConfigDir: t.TempDir(), PlainText: true})
	require.NoError(t, err)
	return c
}

// admiral-state takes every credential as Bearer, an API key included, and
// the workspace is addressed the way a cloud block names it.
func TestListStateVersionsSendsTheKeyAsBearer(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	c := stateClientFor(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotQuery = r.Header.Get("Authorization"), r.URL.EscapedPath(), r.URL.RawQuery
		_, _ = io.WriteString(w, `{"versions":[{"id":"stv-1","serial":7,"writer":{"job":{"job_id":"job-1","attempt":2}},"size_bytes":10}],"next_page_token":"n"}`)
	})

	versions, next, err := c.ListStateVersions(context.Background(), "acme", "shop_prod_net", 50, "p")
	require.NoError(t, err)
	assert.Equal(t, "Bearer admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG", gotAuth)
	assert.Equal(t, "/v1/organizations/acme/workspaces/shop_prod_net/state-versions", gotPath)
	assert.Equal(t, "page_size=50&page_token=p", gotQuery)
	require.Len(t, versions, 1)
	assert.Equal(t, int64(7), versions[0].Serial)
	assert.Equal(t, "job-1", versions[0].Writer.Job.JobID)
	assert.Equal(t, "n", next)
}

func TestAStateErrorCarriesItsDetail(t *testing.T) {
	c := stateClientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","title":"not found","detail":"no hosted state by that name"}]}`)
	})
	_, err := c.DownloadStateVersion(context.Background(), "acme", "shop_prod_net", "stv-1")
	assert.True(t, errors.Is(err, ErrStateNotFound))
	assert.ErrorContains(t, err, "no hosted state by that name")
}

// An API key cannot be refreshed, so a 401 is the answer, sent once.
func TestAnAPIKeyIsNotRetriedOn401(t *testing.T) {
	calls := 0
	c := stateClientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, _, err := c.ListStateVersions(context.Background(), "acme", "shop_prod_net", 0, "")
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}
