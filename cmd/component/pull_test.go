package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

func TestParseSourceReadsTheKindOffTheForm(t *testing.T) {
	src, err := parseSource("cert-manager", sourceFlags{repo: "https://charts.jetstack.io", version: "v1.16.2"})
	require.NoError(t, err)
	helm := src.GetHelmChart()
	require.NotNil(t, helm)
	assert.Equal(t, "https://charts.jetstack.io", helm.Repository)
	assert.Equal(t, "cert-manager", helm.Chart)
	assert.Equal(t, "v1.16.2", helm.Version)

	src, err = parseSource("oci://ghcr.io/argoproj/argo-helm/argo-cd", sourceFlags{version: "7.7.0"})
	require.NoError(t, err)
	require.NotNil(t, src.GetOciChart())
	assert.Equal(t, "7.7.0", src.GetOciChart().Version)

	for _, addr := range []string{
		"GoogleCloudPlatform/cloud-armor/google",
		"terraform-google-modules/log-export/google//modules/storage",
		"registry.acme.example/ns/net/aws",
		"registry.acme.example:8443/ns/net/aws//sub",
	} {
		src, err = parseSource(addr, sourceFlags{version: "~> 8.0"})
		require.NoError(t, err, addr)
		require.NotNil(t, src.GetRegistryModule(), addr)
		assert.Equal(t, addr, src.GetRegistryModule().Address)
		assert.Equal(t, "~> 8.0", src.GetRegistryModule().Version)
	}

	for _, u := range []string{
		"https://github.com/acme/infra.git",
		"https://github.com/acme/infra",
		"git@github.com:acme/infra.git",
		"ssh://git@github.acme.example/acme/infra.git",
		"git::https://git.acme.example/infra.git",
	} {
		src, err = parseSource(u, sourceFlags{ref: "v2", path: "modules/vpc"})
		require.NoError(t, err, u)
		git := src.GetGitTree()
		require.NotNil(t, git, u)
		assert.NotContains(t, git.Url, "git::")
		assert.Contains(t, git.Url, "://", "the API takes a URI")
		assert.Equal(t, "v2", git.Ref)
		assert.Equal(t, "modules/vpc", git.Path)
	}

	src, err = parseSource("git@github.com:admiral-io/admiral-infra.git", sourceFlags{})
	require.NoError(t, err)
	assert.Equal(t, "ssh://git@github.com/admiral-io/admiral-infra.git", src.GetGitTree().Url)
	src, err = parseSource("deploy@git.acme.example:/srv/infra.git", sourceFlags{})
	require.NoError(t, err)
	assert.Equal(t, "ssh://deploy@git.acme.example/srv/infra.git", src.GetGitTree().Url)

	src, err = parseSource("https://github.com/acme/infra/archive/refs/tags/v1.tar.gz", sourceFlags{})
	require.NoError(t, err)
	require.NotNil(t, src.GetArchive())
	src, err = parseSource("https://host/mod.zip?x=1", sourceFlags{})
	require.NoError(t, err)
	require.NotNil(t, src.GetArchive())
}

func TestParseSourceRefusesWhatDoesNotFit(t *testing.T) {
	cases := []struct {
		arg  string
		f    sourceFlags
		want string
	}{
		{"oci://ghcr.io/x/y", sourceFlags{}, "--version is required"},
		{"cert-manager", sourceFlags{repo: "https://charts.jetstack.io"}, "--version is required"},
		{"oci://ghcr.io/x/y", sourceFlags{version: "1", ref: "main"}, "--ref and --path apply to a git repository"},
		{"https://github.com/acme/infra.git", sourceFlags{version: "1"}, "--ref, not --version"},
		{"https://github.com/acme/infra.git", sourceFlags{repo: "https://x"}, "the source is the chart's name"},
		{"a/b/c", sourceFlags{ref: "main"}, "--ref and --path apply to a git repository"},
		{"https://host/mod.tgz", sourceFlags{version: "1"}, "no --version"},
		{"https://example.com/something", sourceFlags{}, "cannot tell what"},
		{"a/b/c/d", sourceFlags{}, "cannot tell what"},
		{"just-a-word", sourceFlags{}, "cannot tell what"},
		{"", sourceFlags{}, "source is required"},
	}
	for _, tc := range cases {
		_, err := parseSource(tc.arg, tc.f)
		require.Error(t, err, tc.arg)
		assert.Contains(t, err.Error(), tc.want, tc.arg)
		assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err), tc.arg)
	}
}

func TestPullRefusesLocallyWhatItCan(t *testing.T) {
	_, err := run(t, "pull", "oci://ghcr.io/x/y", "--version", "1", "--name", "Not_Valid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase letters, digits and hyphens")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "pull")
	require.EqualError(t, err, "missing argument: pull <source>")
}
