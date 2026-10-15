package changeset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/cmderr"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// fakeClient knows one application, shop, with one environment, dev, and
// answers ListChangeSets from open.
type fakeClient struct {
	sdkclient.AdmiralClient
	cs *changeSetClient
}

func (f *fakeClient) Application() applicationv1.ApplicationAPIClient { return appClient{} }
func (f *fakeClient) Environment() environmentv1.EnvironmentAPIClient { return envClient{} }
func (f *fakeClient) ChangeSet() changesetv1.ChangeSetAPIClient       { return f.cs }

const (
	shopID = "app-7f2a1c9d0e3b"
	devID  = "env-3k9x2m7qpl0a"
)

type appClient struct {
	applicationv1.ApplicationAPIClient
}

func (appClient) ListApplications(context.Context, *applicationv1.ListApplicationsRequest, ...grpc.CallOption) (*applicationv1.ListApplicationsResponse, error) {
	return &applicationv1.ListApplicationsResponse{Applications: []*applicationv1.Application{{Id: shopID, Name: "shop"}}}, nil
}

type envClient struct {
	environmentv1.EnvironmentAPIClient
}

func (envClient) ListEnvironments(context.Context, *environmentv1.ListEnvironmentsRequest, ...grpc.CallOption) (*environmentv1.ListEnvironmentsResponse, error) {
	return &environmentv1.ListEnvironmentsResponse{Environments: []*environmentv1.Environment{{Id: devID, Name: "dev"}}}, nil
}

type changeSetClient struct {
	changesetv1.ChangeSetAPIClient
	open []*changesetv1.ChangeSet
	req  *changesetv1.ListChangeSetsRequest
}

func (m *changeSetClient) ListChangeSets(_ context.Context, req *changesetv1.ListChangeSetsRequest, _ ...grpc.CallOption) (*changesetv1.ListChangeSetsResponse, error) {
	m.req = req
	return &changesetv1.ListChangeSetsResponse{ChangeSets: m.open}, nil
}

func resolveWith(t *testing.T, ref string, open ...*changesetv1.ChangeSet) (string, *changeSetClient, error) {
	t.Helper()
	r, err := parseChangeSet(ref)
	require.NoError(t, err)
	cs := &changeSetClient{open: open}
	id, err := r.resolve(context.Background(), &fakeClient{cs: cs})
	return id, cs, err
}

// An ID is used as given, with no lookup.
func TestResolveAnIDAsksNothing(t *testing.T) {
	id, cs, err := resolveWith(t, csID)
	require.NoError(t, err)
	require.Equal(t, csID, id)
	require.Nil(t, cs.req)
}

// app/env resolves to the environment's one open changeset, asked for by
// status so an applied or closed one never counts.
func TestResolveTheOneOpenChangeSet(t *testing.T) {
	id, cs, err := resolveWith(t, "shop/dev", &changesetv1.ChangeSet{Id: csID, Title: "First deploy"})
	require.NoError(t, err)
	require.Equal(t, csID, id)
	require.Equal(t, devID, cs.req.GetEnvironmentId())
	require.Contains(t, cs.req.GetFilter(), "STATUS_OPEN")
}

func TestResolveNoOpenChangeSetSaysHowToStartOne(t *testing.T) {
	_, _, err := resolveWith(t, "shop/dev")
	require.EqualError(t, err, "shop/dev has no open changeset")
	require.Contains(t, cmderr.Hint(err), "admiral changeset create shop/dev")
}

// Several open changesets are listed, never guessed between: a guess
// would apply someone else's change.
func TestResolveSeveralOpenChangeSetsRefuses(t *testing.T) {
	_, _, err := resolveWith(t, "shop/dev",
		&changesetv1.ChangeSet{Id: "cs-aaaaaaaaaaaa", Title: "Add orders"},
		&changesetv1.ChangeSet{Id: "cs-bbbbbbbbbbbb", Title: "Bump api", Behind: true},
	)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "shop/dev has 2 open changesets; name one:")
	require.Contains(t, msg, `cs-aaaaaaaaaaaa  "Add orders"`)
	require.Contains(t, msg, `cs-bbbbbbbbbbbb  "Bump api"`)
	require.Contains(t, msg, "(behind)")
}
