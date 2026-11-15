package changeset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func source(origin changesetv1.ValueSource_Origin, keys ...string) *changesetv1.ValueSource {
	segs := make([]*changesetv1.PathSegment, 0, len(keys))
	for _, k := range keys {
		segs = append(segs, &changesetv1.PathSegment{Segment: &changesetv1.PathSegment_Key{Key: k}})
	}
	return &changesetv1.ValueSource{Path: segs, Origin: origin}
}

// Each value names the layer it came from: a scalar on its line, a map or
// list on its key.
func TestEffectiveValuesLabelEachValueWithItsLayer(t *testing.T) {
	tree := map[string]any{
		"replicas": json.Number("3"),
		"image":    map[string]any{"tag": "1.4.2", "repository": "ghcr.io/acme/api"},
		"hosts":    []any{"api.example.com"},
		"affinity": map[string]any{},
		"envFrom":  []any{},
	}
	out, err := renderEffective(tree, csID, "api", []*changesetv1.ValueSource{
		source(changesetv1.ValueSource_ORIGIN_ENVIRONMENT, "replicas"),
		source(changesetv1.ValueSource_ORIGIN_CHANGE_SET, "image", "tag"),
		source(changesetv1.ValueSource_ORIGIN_COMPONENT_DEFAULT, "image", "repository"),
		source(changesetv1.ValueSource_ORIGIN_BASELINE, "hosts"),
		source(changesetv1.ValueSource_ORIGIN_COMPONENT_DEFAULT, "affinity"),
		source(changesetv1.ValueSource_ORIGIN_COMPONENT_DEFAULT, "envFrom"),
	})
	require.NoError(t, err)
	got := string(out)
	assert.Contains(t, got, "replicas: 3 # environment\n")
	assert.Contains(t, got, "  tag: 1.4.2 # changeset\n")
	assert.Contains(t, got, "  repository: ghcr.io/acme/api # component default\n")
	assert.Contains(t, got, "hosts: # baseline\n")
	assert.Contains(t, got, "affinity: {} # component default\n")
	assert.Contains(t, got, "envFrom: [] # component default\n")

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(out, &parsed), "the labelled file is still YAML:\n%s", got)
	assert.Equal(t, map[string]any{}, parsed["affinity"])
	assert.Contains(t, got, "# changeset: "+csID+"\n# component: api\n")
	assert.NotContains(t, got, "# revision:", "an effective file carries no revision, so an upload refuses it")
}

// The printed file is for reading: an upload of it is refused before the
// network, as for any file with no revision.
func TestAnEffectiveFileCannotBeUploaded(t *testing.T) {
	out, err := renderEffective(map[string]any{"replicas": json.Number("3")}, csID, "api",
		[]*changesetv1.ValueSource{source(changesetv1.ValueSource_ORIGIN_CHANGE_SET, "replicas")})
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "effective.yaml")
	require.NoError(t, os.WriteFile(p, out, 0o644))

	_, err = run(t, "values", csID, "api", "--values", p)
	requireUsage(t, err, "has no changeset, component and revision header")
}

func TestEffectiveAndValuesAreRefusedTogether(t *testing.T) {
	_, err := run(t, "values", csID, "api", "--effective", "--values", "api.yaml")
	requireUsage(t, err, "--effective prints values; it cannot be combined with --values")

	_, err = run(t, "values", csID, "api", "--effective")
	requireAccepted(t, err, "values", csID, "api", "--effective")
}
