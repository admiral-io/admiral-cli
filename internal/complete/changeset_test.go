package complete

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/client"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func (f *fakeClient) ChangeSet() changesetv1.ChangeSetAPIClient { return f.css }

// changeSetClient answers GetChangeSet with one change set in env-1 whose
// head revision adds "worker".
type changeSetClient struct {
	changesetv1.ChangeSetAPIClient
}

func (m *changeSetClient) GetChangeSet(_ context.Context, _ *changesetv1.GetChangeSetRequest, _ ...grpc.CallOption) (*changesetv1.GetChangeSetResponse, error) {
	return &changesetv1.GetChangeSetResponse{
		ChangeSet: &changesetv1.ChangeSet{Id: "cs-000000000001", EnvironmentId: "env-1"},
		Head:      &changesetv1.Revision{Entries: []*changesetv1.Entry{{Component: "worker"}, {Component: "api"}}},
	}, nil
}

// ListEnvironmentComponents serves api and web as live in env-1.
func (m *envClient) ListEnvironmentComponents(_ context.Context, req *environmentv1.ListEnvironmentComponentsRequest, _ ...grpc.CallOption) (*environmentv1.ListEnvironmentComponentsResponse, error) {
	if req.GetEnvironmentId() != "env-1" {
		return &environmentv1.ListEnvironmentComponentsResponse{}, nil
	}
	return &environmentv1.ListEnvironmentComponentsResponse{Components: []*environmentv1.EnvironmentComponent{{Name: "api"}, {Name: "web"}}}, nil
}

func useShop(t *testing.T) {
	t.Helper()
	apps, envs := shopWithEnvs()
	envs.envs[0].Id = "env-1"
	useAll(t, &fakeClient{apps: apps, envs: envs, css: &changeSetClient{}}, nil)
}

func TestPositions_CompletesEachArgumentWithItsOwnFunction(t *testing.T) {
	offer := func(v string) Func {
		return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{v}, cobra.ShellCompDirectiveNoFileComp
		}
	}
	f := Positions(offer("first"), nil, offer("third"))
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"first"}},
		{[]string{"a"}, nil},
		{[]string{"a", "b"}, []string{"third"}},
		{[]string{"a", "b", "c"}, nil},
	} {
		got, directive := f(&cobra.Command{}, tc.args, "")
		require.Equal(t, tc.want, got, "args %v", tc.args)
		require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	}
}

func TestThenEach_RepeatsTheSecondFunction(t *testing.T) {
	f := ThenEach(Static("shop/"), Static("api."))
	got, _ := f(&cobra.Command{}, nil, "")
	require.Equal(t, []string{"shop/"}, got)
	got, _ = f(&cobra.Command{}, []string{"shop/prod", "api.replicas=2"}, "")
	require.Equal(t, []string{"api."}, got)
}

func TestComponentsOf_AnEnvironmentOffersItsLiveComponents(t *testing.T) {
	useShop(t)
	got, directive := ComponentsOf(&client.Options{}, false)(&cobra.Command{}, []string{"shop/prod"}, "")
	require.Equal(t, []string{"api", "web"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentsOf_AChangeSetAddsWhatItsHeadAdds(t *testing.T) {
	useShop(t)
	got, _ := ComponentsOf(&client.Options{}, false)(&cobra.Command{}, []string{"cs-000000000001"}, "")
	require.Equal(t, []string{"api", "web", "worker"}, got)
}

// A set path is typed component first, so the component is offered with
// its dot and no space, and nothing is offered once the dot is there.
func TestComponentsOf_DottedOffersThePathStart(t *testing.T) {
	useShop(t)
	f := ComponentsOf(&client.Options{}, true)
	got, directive := f(&cobra.Command{}, []string{"shop/prod"}, "a")
	require.Equal(t, []string{"api."}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace, directive)

	got, _ = f(&cobra.Command{}, []string{"shop/prod"}, "api.rep")
	require.Nil(t, got)
}

func TestScopes(t *testing.T) {
	useShop(t)
	f := Scopes(&client.Options{})

	got, directive := f(&cobra.Command{}, nil, "")
	require.Equal(t, []string{"org", "app:", "env:", "group:"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace, directive)

	got, directive = f(&cobra.Command{}, nil, "o")
	require.Equal(t, []string{"org"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive, "org takes a space")

	got, _ = f(&cobra.Command{}, nil, "app:sh")
	require.Equal(t, []string{"app:shop\tStorefront", "app:shipping"}, got)

	got, directive = f(&cobra.Command{}, nil, "env:sho")
	require.Equal(t, []string{"env:shop/\tStorefront"}, got)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace, directive)

	got, _ = f(&cobra.Command{}, nil, "env:shop/p")
	require.Equal(t, []string{"env:shop/prod\tProduction"}, got)
}
