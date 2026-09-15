package complete

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/client"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

type fakeClient struct {
	sdkclient.AdmiralClient
	apps *appClient
}

func (f *fakeClient) Application() applicationv1.ApplicationAPIClient { return f.apps }
func (f *fakeClient) Close() error                                    { return nil }

type appClient struct {
	applicationv1.ApplicationAPIClient
	pages [][]*applicationv1.Application // one entry per page
	calls int
	err   error
}

func (m *appClient) ListApplications(_ context.Context, req *applicationv1.ListApplicationsRequest, _ ...grpc.CallOption) (*applicationv1.ListApplicationsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	i := 0
	if req.PageToken != "" {
		_, _ = fmt.Sscanf(req.PageToken, "p%d", &i)
	}
	m.calls++
	resp := &applicationv1.ListApplicationsResponse{Applications: m.pages[i]}
	if i+1 < len(m.pages) {
		resp.NextPageToken = fmt.Sprintf("p%d", i+1)
	}
	return resp, nil
}

func useClient(t *testing.T, apps *appClient, err error) {
	t.Helper()
	prev := newClient
	newClient = func(context.Context, *client.Options) (sdkclient.AdmiralClient, error) {
		if err != nil {
			return nil, err
		}
		return &fakeClient{apps: apps}, nil
	}
	t.Cleanup(func() { newClient = prev })
}

func app(name, desc string) *applicationv1.Application {
	return &applicationv1.Application{Name: name, Description: desc}
}

func TestFilter(t *testing.T) {
	items := []Candidate{{"billing-api", "Billing"}, {"billing-worker", ""}, {"shop", "Storefront"}}
	require.Equal(t, []string{"billing-api\tBilling", "billing-worker"}, Filter(items, "bill"))
	require.Equal(t, []string{"billing-api\tBilling", "billing-worker", "shop\tStorefront"}, Filter(items, ""))
	require.Nil(t, Filter(items, "zzz"))
}

func TestApps_OffersMatchingNames(t *testing.T) {
	useClient(t, &appClient{pages: [][]*applicationv1.Application{{app("billing-api", "Billing"), app("shop", "")}}}, nil)
	got, directive := Apps(&client.Options{})(&cobra.Command{}, nil, "bil")
	require.Equal(t, []string{"billing-api\tBilling"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestApps_FollowsPages(t *testing.T) {
	ac := &appClient{pages: [][]*applicationv1.Application{{app("a", "")}, {app("b", "")}, {app("c", "")}}}
	useClient(t, ac, nil)
	got, _ := Apps(&client.Options{})(&cobra.Command{}, nil, "")
	require.Equal(t, []string{"a", "b", "c"}, got)
	require.Equal(t, 3, ac.calls)
}

// Not being logged in, or the server being down, must be silent: no
// candidates and no file completion fallback.
func TestApps_FailuresAreSilent(t *testing.T) {
	useClient(t, nil, errors.New("not authenticated"))
	got, directive := Apps(&client.Options{})(&cobra.Command{}, nil, "")
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveError, directive)

	useClient(t, &appClient{err: errors.New("unavailable")}, nil)
	got, directive = Apps(&client.Options{})(&cobra.Command{}, nil, "")
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveError, directive)
}

func TestFirst_OnlyCompletesFirstPositional(t *testing.T) {
	called := false
	f := First(func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		called = true
		return []string{"x"}, cobra.ShellCompDirectiveNoFileComp
	})
	got, directive := f(&cobra.Command{}, []string{"already"}, "")
	require.False(t, called)
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	got, _ = f(&cobra.Command{}, nil, "")
	require.True(t, called)
	require.Equal(t, []string{"x"}, got)
}

func TestFlag_PanicsOnUnknownFlag(t *testing.T) {
	require.Panics(t, func() { Flag(&cobra.Command{}, "nope", First(nil)) })
}
