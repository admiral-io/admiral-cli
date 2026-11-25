package component

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func imported(id, ref string) *registryv1.Version {
	return &registryv1.Version{Id: id, Digest: "sha256:" + id + "000000000000", Provenance: &registryv1.Provenance{Kind: registryv1.Provenance_KIND_IMPORT, Ref: ref}}
}

func listed(names ...string) []*sourcev1.SourceVersion {
	out := make([]*sourcev1.SourceVersion, 0, len(names))
	for _, n := range names {
		out = append(out, &sourcev1.SourceVersion{Name: n, Kind: sourcev1.SourceVersion_KIND_VERSION})
	}

	return out
}

// Newer means a released version above every version imported, the way
// latest picks one: branches, pre-releases and names that are not semver
// never count, and the upstream's own spelling is kept.
func TestNewerUpstream(t *testing.T) {
	branch := &sourcev1.SourceVersion{Name: "main", Kind: sourcev1.SourceVersion_KIND_BRANCH}

	testCases := []struct {
		name     string
		upstream []*sourcev1.SourceVersion
		imported []*registryv1.Version
		want     []string
	}{
		{
			name:     "only what is above the newest imported, highest first",
			upstream: listed("2.39.2", "2.40.1", "2.39.4", "2.40.0"),
			imported: []*registryv1.Version{imported("a", "2.39.4"), imported("b", "2.39.2")},
			want:     []string{"2.40.1", "2.40.0"},
		},
		{
			name:     "pre-releases, branches and non-semver names are left out",
			upstream: append(listed("2.41.0-rc.1", "nightly", "2.40.0"), branch),
			imported: []*registryv1.Version{imported("a", "2.39.4")},
			want:     []string{"2.40.0"},
		},
		{
			name:     "a v prefix on either side compares the same",
			upstream: listed("v1.16.3", "v1.16.2"),
			imported: []*registryv1.Version{imported("a", "1.16.2")},
			want:     []string{"v1.16.3"},
		},
		{
			name:     "nothing newer",
			upstream: listed("2.39.4"),
			imported: []*registryv1.Version{imported("a", "2.39.4")},
			want:     []string{},
		},
		{
			name:     "a component imported only from a branch has every release newer",
			upstream: listed("1.0.0"),
			imported: []*registryv1.Version{imported("a", "main")},
			want:     []string{"1.0.0"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, newerUpstream(tc.upstream, tc.imported))
		})
	}
}

func TestVersionName(t *testing.T) {
	assert.Equal(t, "2.39.4", versionName(imported("a", "2.39.4")), "an import is named as the upstream spelled it")
	assert.Equal(t, "v1,latest", versionName(&registryv1.Version{Tags: []string{"v1", "latest"}, Provenance: &registryv1.Provenance{Kind: registryv1.Provenance_KIND_PUSH, Ref: "main"}}), "a push is named by its tags, not its branch")
	assert.Equal(t, "3f9a1c0d2e4b", versionName(&registryv1.Version{Digest: "sha256:3f9a1c0d2e4b5566"}))
}

func render(t *testing.T, comp *registryv1.Component, sec componentSections) string {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, describeComponent(comp, sec).Render(&out))

	return out.String()
}

func TestDescribeComponent(t *testing.T) {
	comp := &registryv1.Component{Id: "cmp-1", Name: "argo-rollouts", Origin: sourceOrigin("src-1", "argo-rollouts")}
	sec := componentSections{
		source:   &sourcev1.Source{Id: "src-1", Name: "argo-helm"},
		versions: []*registryv1.Version{imported("ver-2", "2.39.4"), imported("ver-1", "2.39.2")},
		runs:     map[string][]string{"ver-2": {"shop/prod", "shop/staging"}, "ver-1": {"shop/dev"}},
		upstream: listed("2.40.1", "2.40.0", "2.39.4"),
	}

	t.Run("origin, where each version runs, and what is newer", func(t *testing.T) {
		out := render(t, comp, sec)
		assert.Contains(t, out, "argo-helm/argo-rollouts")
		assert.Regexp(t, `2\.39\.4\s+ver-2000000\S*\s+\S+\s+\S*\s*shop/prod, shop/staging`, out)
		assert.Contains(t, out, "shop/dev")
		assert.Contains(t, out, "Newer In argo-helm")
		assert.Contains(t, out, "2.40.1, 2.40.0")
		assert.Contains(t, out, "admiral component import argo-rollouts --version 2.40.1")
	})

	t.Run("a failed read is shown as unavailable and hides nothing else", func(t *testing.T) {
		broken := sec
		broken.upstreamErr = errors.New("upstream unreachable")
		broken.source = nil
		out := render(t, comp, broken)
		assert.Contains(t, out, "Newer Upstream")
		assert.Contains(t, out, "upstream unreachable")
		assert.Contains(t, out, "shop/prod, shop/staging", "versions still render")
	})

	t.Run("a component published by hand has no upstream section", func(t *testing.T) {
		hand := &registryv1.Component{Id: "cmp-2", Name: "cloud-sql", Origin: publishedOrigin}
		out := render(t, hand, componentSections{versions: []*registryv1.Version{{Id: "ver-9", Tags: []string{"v1.2.0"}, Digest: "sha256:ab"}}})
		assert.Contains(t, out, "published")
		assert.NotContains(t, out, "Newer")
		assert.Contains(t, out, "v1.2.0")
	})

	t.Run("environments the caller cannot read are counted", func(t *testing.T) {
		counted := sec
		counted.unreadableEnvs = 2
		assert.Contains(t, render(t, comp, counted), "2 more environment(s) you cannot read may run it")
	})
}
