package client

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"go.admiral.io/cli/internal/credentials"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

// recordedCall is what the fake server saw for one RPC.
type recordedCall struct {
	method      string
	authz       []string // every authorization header value, in order
	hasDeadline bool
}

// fakeAPI serves ApplicationAPI in-process and returns scripted errors in
// order, then succeeds. It records every call.
type fakeAPI struct {
	applicationv1.UnimplementedApplicationAPIServer
	mu     sync.Mutex
	script map[string][]error // per method, consumed one per call
	calls  []recordedCall
}

func (f *fakeAPI) record(ctx context.Context, method string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	_, hasDL := ctx.Deadline()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedCall{method: method, authz: md.Get("authorization"), hasDeadline: hasDL})
	if errs := f.script[method]; len(errs) > 0 {
		err := errs[0]
		f.script[method] = errs[1:]
		return err
	}
	return nil
}

func (f *fakeAPI) ListApplications(ctx context.Context, _ *applicationv1.ListApplicationsRequest) (*applicationv1.ListApplicationsResponse, error) {
	if err := f.record(ctx, "ListApplications"); err != nil {
		return nil, err
	}
	return &applicationv1.ListApplicationsResponse{}, nil
}

func (f *fakeAPI) CreateApplication(ctx context.Context, _ *applicationv1.CreateApplicationRequest) (*applicationv1.CreateApplicationResponse, error) {
	if err := f.record(ctx, "CreateApplication"); err != nil {
		return nil, err
	}
	return &applicationv1.CreateApplicationResponse{}, nil
}

// startServer runs fakeAPI on a bufconn listener and points CreateClient at it.
func startServer(t *testing.T, api *fakeAPI) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	applicationv1.RegisterApplicationAPIServer(srv, api)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	prev := testDialOptions
	testDialOptions = []grpc.DialOption{grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	})}
	t.Cleanup(func() { testDialOptions = prev })
}

// refreshServer answers refresh_token grants with a fixed new access token.
func refreshServer(t *testing.T, newAccess string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": newAccess, "token_type": "Bearer", "refresh_token": "r2", "expires_in": 3600,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sessionOpts(t *testing.T, tokenURL string) *Options {
	t.Helper()
	dir := t.TempDir()
	os.Unsetenv(credentials.EnvAPIKey)
	require.NoError(t, credentials.Save(dir, &credentials.Credential{
		Kind: credentials.KindSession, AccessToken: "jwt-1", RefreshToken: "r1",
		Expiry: time.Now().Add(time.Hour), TokenURL: tokenURL, ClientID: "admiral-cli",
	}))
	return &Options{ServerAddr: "passthrough:///bufconn", PlainText: true, ConfigDir: dir}
}

func TestCreateClient_SessionRefreshOn401(t *testing.T) {
	api := &fakeAPI{script: map[string][]error{
		"ListApplications": {status.Error(codes.Unauthenticated, "token expired")},
	}}
	startServer(t, api)
	refresh := refreshServer(t, "jwt-2")
	opts := sessionOpts(t, refresh.URL)

	c, err := CreateClient(context.Background(), opts)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.NoError(t, err, "one 401 is recovered by refreshing")

	require.Len(t, api.calls, 2)
	require.Equal(t, []string{"Bearer jwt-1"}, api.calls[0].authz, "first attempt carries the stored session token")
	require.Equal(t, []string{"Bearer jwt-2"}, api.calls[1].authz, "retry carries exactly one header, the refreshed token")
	for _, call := range api.calls {
		require.True(t, call.hasDeadline, "every call reaches the server with a deadline")
	}

	// Later calls on the same client use the refreshed token too.
	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer jwt-2"}, api.calls[2].authz)

	// The refreshed session was persisted for the next invocation.
	cred, err := credentials.Load(opts.ConfigDir)
	require.NoError(t, err)
	require.Equal(t, "jwt-2", cred.AccessToken)
}

func TestCreateClient_SessionRefreshOnlyOnce(t *testing.T) {
	api := &fakeAPI{script: map[string][]error{
		"ListApplications": {status.Error(codes.Unauthenticated, "no"), status.Error(codes.Unauthenticated, "still no")},
	}}
	startServer(t, api)
	opts := sessionOpts(t, refreshServer(t, "jwt-2").URL)

	c, err := CreateClient(context.Background(), opts)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err), "a second 401 is returned, not retried again")
	require.Len(t, api.calls, 2)
}

func TestCreateClient_APIKeyUsesTokenScheme(t *testing.T) {
	api := &fakeAPI{script: map[string][]error{}}
	startServer(t, api)
	t.Setenv(credentials.EnvAPIKey, testToken)
	opts := &Options{ServerAddr: "bufconn:1", PlainText: true, ConfigDir: t.TempDir()}

	c, err := CreateClient(context.Background(), opts)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{"Token " + testToken}, api.calls[0].authz)
	require.True(t, api.calls[0].hasDeadline)
}

func TestCreateClient_TransientRetryReadsOnly(t *testing.T) {
	prev := retryBackoff
	retryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryBackoff = prev })

	api := &fakeAPI{script: map[string][]error{
		"ListApplications":  {status.Error(codes.Unavailable, "blip")},
		"CreateApplication": {status.Error(codes.Unavailable, "blip")},
	}}
	startServer(t, api)
	t.Setenv(credentials.EnvAPIKey, testToken)
	opts := &Options{ServerAddr: "bufconn:1", PlainText: true, ConfigDir: t.TempDir()}

	c, err := CreateClient(context.Background(), opts)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.NoError(t, err, "a read recovers from one Unavailable")

	_, err = c.Application().CreateApplication(context.Background(), &applicationv1.CreateApplicationRequest{Name: "x"})
	require.Equal(t, codes.Unavailable, status.Code(err), "a mutation is never retried")

	var lists, creates int
	for _, call := range api.calls {
		switch call.method {
		case "ListApplications":
			lists++
		case "CreateApplication":
			creates++
		}
	}
	require.Equal(t, 2, lists)
	require.Equal(t, 1, creates)
}

func TestCreateClient_TimeoutOptionIsApplied(t *testing.T) {
	api := &fakeAPI{script: map[string][]error{}}
	startServer(t, api)
	t.Setenv(credentials.EnvAPIKey, testToken)
	opts := &Options{ServerAddr: "bufconn:1", PlainText: true, ConfigDir: t.TempDir(), Timeout: 2 * time.Second}

	c, err := CreateClient(context.Background(), opts)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
	require.NoError(t, err)
	require.True(t, api.calls[0].hasDeadline, "the configured timeout reaches the server as a deadline")
}

// -v output is produced through slog on both credential paths, and each
// retry attempt is its own line.
func TestCreateClient_VerboseLogsEveryAttemptOnBothPaths(t *testing.T) {
	prev := retryBackoff
	retryBackoff = []time.Duration{time.Millisecond}
	t.Cleanup(func() { retryBackoff = prev })

	var buf strings.Builder
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	for name, mk := range map[string]func(t *testing.T) *Options{
		"api-key": func(t *testing.T) *Options {
			t.Setenv(credentials.EnvAPIKey, testToken)
			return &Options{ServerAddr: "bufconn:1", PlainText: true, ConfigDir: t.TempDir(), Verbose: true}
		},
		"session": func(t *testing.T) *Options {
			o := sessionOpts(t, "")
			o.Verbose = true
			return o
		},
	} {
		t.Run(name, func(t *testing.T) {
			buf.Reset()
			api := &fakeAPI{script: map[string][]error{"ListApplications": {status.Error(codes.Unavailable, "blip")}}}
			startServer(t, api)
			c, err := CreateClient(context.Background(), mk(t))
			require.NoError(t, err)
			defer c.Close() //nolint:errcheck

			_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
			require.NoError(t, err)

			out := buf.String()
			require.Contains(t, out, "connecting")
			require.Contains(t, out, "auth="+name)
			require.Equal(t, 2, strings.Count(out, "msg=rpc"), "one line per attempt: %s", out)
			require.Contains(t, out, "code=Unavailable")
			require.Contains(t, out, "code=OK")
			require.Contains(t, out, "ListApplications")
		})
	}
}
