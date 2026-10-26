package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
// the history is the documented state-versions list.
func TestListStateVersionsReadsTheDocumentedList(t *testing.T) {
	var gotAuth, gotPath string
	var gotQuery url.Values
	c := stateClientFor(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotQuery = r.Header.Get("Authorization"), r.URL.Path, r.URL.Query()
		_, _ = io.WriteString(w, `{"data":[{"id":"stv-1","type":"state-versions","attributes":{"serial":7,"size":10,"forced":true},`+
			`"relationships":{"created-by":{"data":{"type":"jobs","id":"job-1"},"meta":{"attempt":2}}}}],"meta":{"pagination":{"next-page":3}}}`)
	})

	versions, next, err := c.ListStateVersions(context.Background(), "acme", "shop_prod_net", 50, "2")
	require.NoError(t, err)
	assert.Equal(t, "Bearer admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG", gotAuth)
	assert.Equal(t, "/api/v2/state-versions", gotPath)
	assert.Equal(t, "acme", gotQuery.Get("filter[organization][name]"))
	assert.Equal(t, "shop_prod_net", gotQuery.Get("filter[workspace][name]"))
	assert.Equal(t, "50", gotQuery.Get("page[size]"))
	assert.Equal(t, "2", gotQuery.Get("page[number]"))
	require.Len(t, versions, 1)
	assert.Equal(t, int64(7), versions[0].Serial)
	assert.True(t, versions[0].Forced)
	assert.Equal(t, StateWriter{Type: "jobs", ID: "job-1", Attempt: 2}, versions[0].CreatedBy)
	assert.Equal(t, "3", next, "the next page number is the next token")
}

func TestAStateErrorCarriesItsDetail(t *testing.T) {
	c := stateClientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","title":"not found","detail":"no hosted state by that name"}]}`)
	})
	_, err := c.DownloadStateVersion(context.Background(), "stv-1")
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
