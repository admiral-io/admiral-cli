package component

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// sourceClient is a SourceAPI with one source, what it offers, and what
// each artifact has.
type sourceClient struct {
	sourcev1.SourceAPIClient
	source    *sourcev1.Source
	artifacts []*sourcev1.SourceArtifact
	versions  []*sourcev1.SourceVersion
	asked     []string // the artifacts versions were listed for
}

func (c *sourceClient) ListSources(_ context.Context, req *sourcev1.ListSourcesRequest, _ ...grpc.CallOption) (*sourcev1.ListSourcesResponse, error) {
	if req.Filter == "" || strings.Contains(req.Filter, c.source.Name) {
		return &sourcev1.ListSourcesResponse{Sources: []*sourcev1.Source{c.source}}, nil
	}

	return &sourcev1.ListSourcesResponse{}, nil
}

func (c *sourceClient) GetSource(_ context.Context, _ *sourcev1.GetSourceRequest, _ ...grpc.CallOption) (*sourcev1.GetSourceResponse, error) {
	return &sourcev1.GetSourceResponse{Source: c.source}, nil
}

func (c *sourceClient) ListSourceArtifacts(_ context.Context, _ *sourcev1.ListSourceArtifactsRequest, _ ...grpc.CallOption) (*sourcev1.ListSourceArtifactsResponse, error) {
	return &sourcev1.ListSourceArtifactsResponse{Artifacts: c.artifacts}, nil
}

func (c *sourceClient) ListSourceVersions(_ context.Context, req *sourcev1.ListSourceVersionsRequest, _ ...grpc.CallOption) (*sourcev1.ListSourceVersionsResponse, error) {
	c.asked = append(c.asked, req.Artifact)
	return &sourcev1.ListSourceVersionsResponse{Versions: c.versions}, nil
}

func sourceOf(kind sourcev1.Source_Kind, location string) *sourcev1.Source {
	return &sourcev1.Source{Id: "src-000000000001", Name: "upstream", Kind: kind, Location: location, Status: sourcev1.Source_STATUS_READY}
}

// scripted is a command whose stdin is answered and whose session is
// interactive when answers is not nil.
func scripted(t *testing.T, answers *string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{}

	var errOut bytes.Buffer

	in := ""
	env := func(string) string { return "" }

	if answers != nil {
		in = *answers
		env = func(k string) string {
			if k == "ADMIRAL_FORCE_INTERACTIVE" {
				return "1"
			}

			return ""
		}
	}

	cmd.SetContext(iostreams.WithStreams(context.Background(), iostreams.New(strings.NewReader(in), &bytes.Buffer{}, &errOut, env)))

	return cmd, &errOut
}

// What is given is taken as is, and the artifact defaults to the
// component's name everywhere but a git repository, whose root is empty.
func TestThroughSourceTakesWhatIsGivenAsIs(t *testing.T) {
	c := &sourceClient{source: sourceOf(sourcev1.Source_KIND_HELM_REPO, "https://charts.jetstack.io")}
	cmd, _ := scripted(t, nil)
	got, err := throughSource(cmd, c, "upstream", "cert-manager", "", "v1.16.2")
	require.NoError(t, err)
	assert.Equal(t, "src-000000000001", got.sourceID)
	assert.Equal(t, "cert-manager", got.artifact, "the artifact defaults to the component's name")
	assert.Equal(t, "v1.16.2", got.version)
	assert.Equal(t, "cert-manager v1.16.2 from upstream", got.label)
	assert.Empty(t, c.asked, "nothing is listed when everything is given")

	got, err = throughSource(cmd, c, "upstream", "certs", "cert-manager", "v1.16.2")
	require.NoError(t, err)
	assert.Equal(t, "cert-manager", got.artifact, "--artifact wins over the name")

	c = &sourceClient{source: sourceOf(sourcev1.Source_KIND_GIT, "https://github.com/acme/infra")}
	got, err = throughSource(cmd, c, "upstream", "vpc", "/modules/vpc/", "v2.1.0")
	require.NoError(t, err)
	assert.Equal(t, "modules/vpc", got.artifact)
	assert.Equal(t, "modules/vpc v2.1.0 from upstream", got.label)
	got, err = throughSource(cmd, c, "upstream", "infra", "", "v2.1.0")
	require.NoError(t, err)
	assert.Equal(t, "", got.artifact, "a git repository's root is the empty path, not the component's name")
	assert.Equal(t, "https://github.com/acme/infra v2.1.0 from upstream", got.label)
}

func TestThroughSourceRefusesWhatItCannotUse(t *testing.T) {
	cmd, _ := scripted(t, nil)

	needs := sourceOf(sourcev1.Source_KIND_HELM_REPO, "https://charts.jetstack.io")
	needs.Status, needs.StatusReason = sourcev1.Source_STATUS_NEEDS_ATTENTION, "the credential no longer reaches it"
	_, err := throughSource(cmd, &sourceClient{source: needs}, "upstream", "cert-manager", "", "v1")
	require.EqualError(t, err, "source upstream needs attention: the credential no longer reaches it")
	assert.Equal(t, cmderr.ExitError, cmderr.Code(err))

	_, err = throughSource(cmd, &sourceClient{source: sourceOf(sourcev1.Source_KIND_UNSPECIFIED, "x")}, "upstream", "a", "", "1")
	require.ErrorContains(t, err, "cannot import from")

	miss := &sourceClient{source: sourceOf(sourcev1.Source_KIND_GIT, "x")}
	_, err = throughSource(cmd, miss, "other", "infra", "", "v1")
	require.EqualError(t, err, `source "other" not found`)
	assert.Equal(t, "Sources: upstream", cmderr.Hint(err))
}

// The version is never guessed: without a terminal the refusal lists what
// the source offers; in one, the list is asked with nothing pre-selected,
// and an empty answer imports nothing.
func TestThroughSourceNeverGuessesTheVersion(t *testing.T) {
	cmd, _ := scripted(t, nil)
	helm := &sourceClient{
		source:   sourceOf(sourcev1.Source_KIND_HELM_REPO, "https://charts.jetstack.io"),
		versions: []*sourcev1.SourceVersion{{Name: "v1.16.2"}, {Name: "v1.16.1"}},
	}
	_, err := throughSource(cmd, helm, "upstream", "cert-manager", "", "")
	require.EqualError(t, err, "--version required when not running interactively")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	assert.Equal(t, "Pass --version as one of: v1.16.2, v1.16.1", cmderr.Hint(err))
	assert.Equal(t, []string{"cert-manager"}, helm.asked)

	git := &sourceClient{
		source: sourceOf(sourcev1.Source_KIND_GIT, "https://github.com/acme/infra"),
		versions: []*sourcev1.SourceVersion{
			{Name: "v2.1.0", Kind: sourcev1.SourceVersion_KIND_VERSION, Commit: "0123456789abcdef"},
			{Name: "main", Kind: sourcev1.SourceVersion_KIND_BRANCH, Commit: "fedcba9876543210"},
		},
	}
	answers := "main\n"
	cmd, errOut := scripted(t, &answers)
	got, err := throughSource(cmd, git, "upstream", "vpc", "modules/vpc", "")
	require.NoError(t, err)
	assert.Equal(t, "main", got.version)
	assert.Contains(t, errOut.String(), "  1. v2.1.0  (version 0123456)\n  2. main  (branch fedcba9)\n")
	assert.Equal(t, []string{""}, git.asked, "a repository's versions do not depend on the path")

	answers = "\n"
	cmd, _ = scripted(t, &answers)
	_, err = throughSource(cmd, git, "upstream", "vpc", "modules/vpc", "")
	require.ErrorContains(t, err, "canceled")
}
