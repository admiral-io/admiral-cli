package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// registryClient is a RegistryAPI that knows one component, or none.
type registryClient struct {
	registryv1.RegistryAPIClient
	component *registryv1.Component
}

func (c *registryClient) GetComponent(_ context.Context, _ *registryv1.GetComponentRequest, _ ...grpc.CallOption) (*registryv1.GetComponentResponse, error) {
	if c.component == nil {
		return nil, status.Error(codes.NotFound, "component not found")
	}
	return &registryv1.GetComponentResponse{Component: c.component}, nil
}

func sourceOrigin(sourceID, artifact string) *registryv1.ComponentOrigin {
	return &registryv1.ComponentOrigin{Kind: &registryv1.ComponentOrigin_Source{Source: &registryv1.SourceOrigin{SourceId: sourceID, Artifact: artifact}}}
}

var publishedOrigin = &registryv1.ComponentOrigin{Kind: &registryv1.ComponentOrigin_Published{Published: &registryv1.PublishedOrigin{}}}

// A bump with no --source goes through the source the component remembers,
// and each way it cannot says what to do instead.
func TestOriginOfIsWhereABumpGoesThrough(t *testing.T) {
	ctx := context.Background()

	id, artifact, err := originOf(ctx, &registryClient{component: &registryv1.Component{Name: "argo-rollouts", Origin: sourceOrigin("src-000000000001", "argo-rollouts")}}, "argo-rollouts")
	require.NoError(t, err)
	assert.Equal(t, "src-000000000001", id)
	assert.Equal(t, "argo-rollouts", artifact)

	_, _, err = originOf(ctx, &registryClient{}, "argo-rollouts")
	require.ErrorContains(t, err, "argo-rollouts is not a component yet; name the source to import it from with --source")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err), "a new name without --source is a usage error")

	_, _, err = originOf(ctx, &registryClient{component: &registryv1.Component{Name: "cert-manager", Origin: publishedOrigin}}, "cert-manager")
	require.ErrorContains(t, err, "cert-manager is published by hand")
	assert.Contains(t, cmderr.Hint(err), "admiral component publish")
	assert.Contains(t, cmderr.Hint(err), "admiral component set-origin cert-manager --source SOURCE")

	_, _, err = originOf(ctx, &registryClient{component: &registryv1.Component{Name: "vpc"}}, "vpc")
	require.ErrorContains(t, err, "vpc has no origin yet")
	assert.Contains(t, cmderr.Hint(err), "--source")
}

func TestOriginLabel(t *testing.T) {
	names := map[string]string{"src-000000000001": "argo-helm"}
	testCases := []struct {
		name   string
		origin *registryv1.ComponentOrigin
		want   string
	}{
		{name: "a source is shown by name with its artifact", origin: sourceOrigin("src-000000000001", "argo-rollouts"), want: "argo-helm/argo-rollouts"},
		{name: "an unnamed source is shown by ID", origin: sourceOrigin("src-000000000002", "vpc"), want: "src-000000000002/vpc"},
		{name: "a git root is the source alone", origin: sourceOrigin("src-000000000001", ""), want: "argo-helm"},
		{name: "published by hand", origin: publishedOrigin, want: "published"},
		{name: "no origin yet is empty", origin: nil, want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, originLabel(tc.origin, names))
		})
	}
}

func TestSetOriginRefusesLocallyWhatItCan(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "neither", args: []string{"set-origin", "x"}, want: "give --source, or --published for a component published by hand"},
		{name: "both", args: []string{"set-origin", "x", "--source", "s", "--published"}, want: "give --source or --published, not both"},
		{name: "an artifact without a source", args: []string{"set-origin", "x", "--published", "--artifact", "a"}, want: "--artifact goes with --source"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.args...)
			require.ErrorContains(t, err, tc.want)
			assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		})
	}
}
