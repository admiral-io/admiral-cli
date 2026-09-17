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
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

type fakeClient struct {
	sdkclient.AdmiralClient
	apps *appClient
	envs *envClient
}

func (f *fakeClient) Application() applicationv1.ApplicationAPIClient { return f.apps }
func (f *fakeClient) Environment() environmentv1.EnvironmentAPIClient { return f.envs }
func (f *fakeClient) Close() error                                    { return nil }

// envClient serves one page of environments and records the filter it
// was asked for, so a test can check the scope.
type envClient struct {
	environmentv1.EnvironmentAPIClient
	envs   []*environmentv1.Environment
	filter string
}

func (m *envClient) ListEnvironments(_ context.Context, req *environmentv1.ListEnvironmentsRequest, _ ...grpc.CallOption) (*environmentv1.ListEnvironmentsResponse, error) {
	m.filter = req.Filter
	return &environmentv1.ListEnvironmentsResponse{Environments: m.envs}, nil
}

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
	useClients(t, apps, &envClient{}, err)
}

func useClients(t *testing.T, apps *appClient, envs *envClient, err error) {
	t.Helper()
	prev := newClient
	newClient = func(context.Context, *client.Options) (sdkclient.AdmiralClient, error) {
		if err != nil {
			return nil, err
		}
		return &fakeClient{apps: apps, envs: envs}, nil
	}
	t.Cleanup(func() { newClient = prev })
}

// scopedCmd is a command with --app registered and, when app is not
// empty, already parsed from the line.
func scopedCmd(t *testing.T, app string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("app", "", "")
	if app != "" {
		require.NoError(t, cmd.Flags().Set("app", app))
	}
	return cmd
}

// shopWithEnvs is one application, shop (id app-1), with two environments.
func shopWithEnvs() (*appClient, *envClient) {
	apps := &appClient{pages: [][]*applicationv1.Application{{
		{Id: "app-1", Name: "shop", Description: "Storefront"},
		{Id: "app-2", Name: "shipping"},
	}}}
	envs := &envClient{envs: []*environmentv1.Environment{
		{Name: "prod", Description: "Production"},
		{Name: "staging"},
	}}
	return apps, envs
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

func TestEnvs_BareNameCompletesInsideAppFlag(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, directive := Envs(&client.Options{})(scopedCmd(t, "shop"), nil, "pr")
	require.Equal(t, []string{"prod\tProduction"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	require.Equal(t, "field['application_id'] = 'app-1'", envs.filter)
}

// With no scope on the line, the only thing to offer is the first segment
// of the path, and the shell must not add a space after it.
func TestEnvs_NoScopeOffersAppPrefixes(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, directive := Envs(&client.Options{})(scopedCmd(t, ""), nil, "sh")
	require.Equal(t, []string{"shop/\tStorefront", "shipping/"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace, directive)
}

func TestEnvs_PathCompletesSecondSegment(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, directive := Envs(&client.Options{})(scopedCmd(t, ""), nil, "shop/st")
	require.Equal(t, []string{"shop/staging"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	require.Equal(t, "field['application_id'] = 'app-1'", envs.filter)

	got, _ = Envs(&client.Options{})(scopedCmd(t, ""), nil, "shop/")
	require.Equal(t, []string{"shop/prod\tProduction", "shop/staging"}, got)
}

// A command without an --app flag (changeset copy --env) still completes.
func TestEnvs_NoAppFlagRegistered(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, _ := Envs(&client.Options{})(&cobra.Command{}, nil, "shop/p")
	require.Equal(t, []string{"shop/prod\tProduction"}, got)
}

func TestEnvs_UnknownAppIsSilent(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, directive := Envs(&client.Options{})(scopedCmd(t, ""), nil, "nope/")
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveError, directive)
}

// A new environment's name cannot be completed; only its application can.
func TestNewEnv_OffersOnlyAppPrefixes(t *testing.T) {
	apps, envs := shopWithEnvs()
	useClients(t, apps, envs, nil)
	got, directive := NewEnv(&client.Options{})(scopedCmd(t, ""), nil, "sh")
	require.Equal(t, []string{"shop/\tStorefront", "shipping/"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace, directive)

	got, directive = NewEnv(&client.Options{})(scopedCmd(t, ""), nil, "shop/")
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	got, directive = NewEnv(&client.Options{})(scopedCmd(t, "shop"), nil, "")
	require.Nil(t, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
