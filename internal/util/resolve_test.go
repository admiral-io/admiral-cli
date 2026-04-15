package util

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
	commonv1 "go.admiral.io/sdk/proto/admiral/common/v1"
	userv1 "go.admiral.io/sdk/proto/admiral/user/v1"
)

// ---------------------------------------------------------------------------
// Mock clients
// ---------------------------------------------------------------------------

type mockAppClient struct {
	applicationv1.ApplicationAPIClient // embed for unused methods
	resp                               *applicationv1.ListApplicationsResponse
	err                                error
}

func (m *mockAppClient) ListApplications(_ context.Context, _ *applicationv1.ListApplicationsRequest, _ ...grpc.CallOption) (*applicationv1.ListApplicationsResponse, error) {
	return m.resp, m.err
}

// ---------------------------------------------------------------------------
// ResolveAppID
// ---------------------------------------------------------------------------

func TestResolveAppID(t *testing.T) {
	ctx := context.Background()

	t.Run("idFlag set returns directly", func(t *testing.T) {
		id, err := ResolveAppID(ctx, nil, "", "some-uuid")
		require.NoError(t, err)
		require.Equal(t, "some-uuid", id)
	})

	t.Run("name found returns ID", func(t *testing.T) {
		client := &mockAppClient{
			resp: &applicationv1.ListApplicationsResponse{
				Applications: []*applicationv1.Application{
					{Id: "app-123", Name: "billing-api"},
				},
			},
		}
		id, err := ResolveAppID(ctx, client, "billing-api", "")
		require.NoError(t, err)
		require.Equal(t, "app-123", id)
	})

	t.Run("name not found", func(t *testing.T) {
		client := &mockAppClient{
			resp: &applicationv1.ListApplicationsResponse{},
		}
		_, err := ResolveAppID(ctx, client, "ghost", "")
		require.ErrorContains(t, err, `application "ghost" not found`)
	})

	t.Run("multiple matches", func(t *testing.T) {
		client := &mockAppClient{
			resp: &applicationv1.ListApplicationsResponse{
				Applications: []*applicationv1.Application{
					{Id: "a1", Name: "dup"}, {Id: "a2", Name: "dup"},
				},
			},
		}
		_, err := ResolveAppID(ctx, client, "dup", "")
		require.ErrorContains(t, err, "multiple applications match")
	})

	t.Run("empty name and empty idFlag", func(t *testing.T) {
		_, err := ResolveAppID(ctx, nil, "", "")
		require.ErrorContains(t, err, "no application name")
	})

	t.Run("RPC error", func(t *testing.T) {
		client := &mockAppClient{err: fmt.Errorf("connection refused")}
		_, err := ResolveAppID(ctx, client, "billing-api", "")
		require.ErrorContains(t, err, "looking up application")
		require.ErrorContains(t, err, "connection refused")
	})
}

// ---------------------------------------------------------------------------
// Mock user client
// ---------------------------------------------------------------------------

type mockUserClient struct {
	userv1.UserAPIClient
	resp *userv1.ListPersonalAccessTokensResponse
	err  error
}

func (m *mockUserClient) ListPersonalAccessTokens(_ context.Context, _ *userv1.ListPersonalAccessTokensRequest, _ ...grpc.CallOption) (*userv1.ListPersonalAccessTokensResponse, error) {
	return m.resp, m.err
}

// ---------------------------------------------------------------------------
// ResolvePersonalAccessTokenID
// ---------------------------------------------------------------------------

func TestResolvePersonalAccessTokenID(t *testing.T) {
	ctx := context.Background()

	t.Run("idFlag set returns directly", func(t *testing.T) {
		id, err := ResolvePersonalAccessTokenID(ctx, nil, "", "pat-uuid")
		require.NoError(t, err)
		require.Equal(t, "pat-uuid", id)
	})

	t.Run("name found returns ID", func(t *testing.T) {
		client := &mockUserClient{
			resp: &userv1.ListPersonalAccessTokensResponse{
				AccessTokens: []*commonv1.AccessToken{
					{Id: "pat-123", Name: "ci-deploy"},
				},
			},
		}
		id, err := ResolvePersonalAccessTokenID(ctx, client, "ci-deploy", "")
		require.NoError(t, err)
		require.Equal(t, "pat-123", id)
	})

	t.Run("client-side filter ignores non-matching names", func(t *testing.T) {
		client := &mockUserClient{
			resp: &userv1.ListPersonalAccessTokensResponse{
				AccessTokens: []*commonv1.AccessToken{
					{Id: "pat-1", Name: "ci-deploy"},
					{Id: "pat-2", Name: "local-dev"},
				},
			},
		}
		id, err := ResolvePersonalAccessTokenID(ctx, client, "ci-deploy", "")
		require.NoError(t, err)
		require.Equal(t, "pat-1", id)
	})

	t.Run("name not found", func(t *testing.T) {
		client := &mockUserClient{
			resp: &userv1.ListPersonalAccessTokensResponse{},
		}
		_, err := ResolvePersonalAccessTokenID(ctx, client, "ghost", "")
		require.ErrorContains(t, err, `token "ghost" not found`)
	})

	t.Run("multiple matches", func(t *testing.T) {
		client := &mockUserClient{
			resp: &userv1.ListPersonalAccessTokensResponse{
				AccessTokens: []*commonv1.AccessToken{
					{Id: "p1", Name: "dup"}, {Id: "p2", Name: "dup"},
				},
			},
		}
		_, err := ResolvePersonalAccessTokenID(ctx, client, "dup", "")
		require.ErrorContains(t, err, "multiple tokens match")
	})

	t.Run("empty name and empty idFlag", func(t *testing.T) {
		_, err := ResolvePersonalAccessTokenID(ctx, nil, "", "")
		require.ErrorContains(t, err, "no token name")
	})

	t.Run("RPC error", func(t *testing.T) {
		client := &mockUserClient{err: fmt.Errorf("unauthorized")}
		_, err := ResolvePersonalAccessTokenID(ctx, client, "my-token", "")
		require.ErrorContains(t, err, "looking up token")
		require.ErrorContains(t, err, "unauthorized")
	})
}
