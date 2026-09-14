package resolve

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/cmderr"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

type appClient struct {
	applicationv1.ApplicationAPIClient
	apps   []*applicationv1.Application
	filter string
	err    error
}

func (m *appClient) ListApplications(_ context.Context, req *applicationv1.ListApplicationsRequest, _ ...grpc.CallOption) (*applicationv1.ListApplicationsResponse, error) {
	m.filter = req.Filter
	return &applicationv1.ListApplicationsResponse{Applications: m.apps}, m.err
}

type envClient struct {
	environmentv1.EnvironmentAPIClient
	envs   []*environmentv1.Environment
	filter string
}

func (m *envClient) ListEnvironments(_ context.Context, req *environmentv1.ListEnvironmentsRequest, _ ...grpc.CallOption) (*environmentv1.ListEnvironmentsResponse, error) {
	m.filter = req.Filter
	return &environmentv1.ListEnvironmentsResponse{Environments: m.envs}, nil
}

const uuid = "550e8400-e29b-41d4-a716-446655440000"

func TestIsUUID(t *testing.T) {
	require.True(t, IsUUID(uuid))
	require.True(t, IsUUID("550E8400-E29B-41D4-A716-446655440000"))
	require.False(t, IsUUID("shop"))
	require.False(t, IsUUID("run-vnx81r"))
	require.False(t, IsUUID(uuid+"x"))
}

func TestApp_UUIDSkipsLookup(t *testing.T) {
	c := &appClient{err: errors.New("must not be called")}
	id, err := App(context.Background(), c, uuid)
	require.NoError(t, err)
	require.Equal(t, uuid, id)
}

func TestApp_Empty(t *testing.T) {
	_, err := App(context.Background(), &appClient{}, "")
	require.EqualError(t, err, "no application specified")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Equal(t, "Pass --app or set ADMIRAL_APP.", cmderr.Hint(err))
}

func TestApp_ByName(t *testing.T) {
	c := &appClient{apps: []*applicationv1.Application{{Id: "id-1", Name: "shop"}, {Id: "id-2", Name: "shop-old"}}}
	id, err := App(context.Background(), c, "shop")
	require.NoError(t, err)
	require.Equal(t, "id-1", id, "client-side re-check ignores prefix matches the server may return")
	require.Equal(t, "field['name'] = 'shop'", c.filter)
}

func TestApp_NotFound(t *testing.T) {
	_, err := App(context.Background(), &appClient{}, "nope")
	require.EqualError(t, err, `application "nope" not found`)
	require.Equal(t, "Run 'admiral app list' to see applications.", cmderr.Hint(err))
	require.Equal(t, cmderr.ExitError, cmderr.Code(err))
	var nf *NotFoundError
	require.ErrorAs(t, err, &nf)
}

func TestApp_Ambiguous(t *testing.T) {
	c := &appClient{apps: []*applicationv1.Application{{Id: "id-1", Name: "shop"}, {Id: "id-2", Name: "shop"}}}
	_, err := App(context.Background(), c, "shop")
	require.EqualError(t, err, `2 applications are named "shop"`)
	require.Equal(t, "Pass an ID instead: id-1, id-2", cmderr.Hint(err))
}

func TestEnvironment_UUIDNeedsNoApp(t *testing.T) {
	id, err := Environment(context.Background(), &envClient{}, &appClient{err: errors.New("no")}, "", uuid)
	require.NoError(t, err)
	require.Equal(t, uuid, id)
}

func TestEnvironment_NameNeedsApp(t *testing.T) {
	_, err := Environment(context.Background(), &envClient{}, &appClient{}, "", "prod")
	require.EqualError(t, err, "no application specified")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func TestEnvironment_ScopedLookup(t *testing.T) {
	apps := &appClient{apps: []*applicationv1.Application{{Id: "app-1", Name: "shop"}}}
	envs := &envClient{envs: []*environmentv1.Environment{{Id: "env-1", Name: "prod", ApplicationId: "app-1"}}}
	id, err := Environment(context.Background(), envs, apps, "shop", "prod")
	require.NoError(t, err)
	require.Equal(t, "env-1", id)
	require.Equal(t, "field['application_id'] = 'app-1' AND field['name'] = 'prod'", envs.filter)

	_, err = Environment(context.Background(), &envClient{}, apps, "shop", "prd")
	require.EqualError(t, err, `environment "prd" not found in application "shop"`)
	require.Equal(t, "Run 'admiral env list --app shop' to see environments.", cmderr.Hint(err))
}
