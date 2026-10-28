package run

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func value(t *testing.T, v map[string]any) *structpb.Value {
	t.Helper()
	sv, err := structpb.NewValue(v)
	require.NoError(t, err)
	return sv
}

func keyPath(keys ...string) *agentv1.DiffPath {
	p := &agentv1.DiffPath{}
	for _, k := range keys {
		p.Segments = append(p.Segments, &agentv1.DiffPathSegment{Segment: &agentv1.DiffPathSegment_Key{Key: k}})
	}
	return p
}

// The deploy that started it: one image tag changed in a Deployment. --full
// shows the whole object as it lands, the tag's line marked and the rest
// kept for context.
func TestFullDiffShowsTheWholeResourceWithItsChangeMarked(t *testing.T) {
	deployment := func(tag string) map[string]any {
		return map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "api", "namespace": "shop"},
			"spec": map[string]any{"replicas": 2, "template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "api", "image": "ghcr.io/shop/api:" + tag, "imagePullPolicy": "IfNotPresent"}},
			}}},
		}
	}
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{{
		Component: "api", Action: agentv1.ResourceChange_ACTION_UPDATE, Resource: k8sRef("Deployment", "api"),
		Before: value(t, deployment("35bd16b")), After: value(t, deployment("15df5e1")),
	}}}
	var b bytes.Buffer
	WriteFullDiff(&b, d, nil)
	assert.Equal(t, `~ api: v1 Deployment podinfo-dev/api (update)
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: api
      namespace: shop
    spec:
      replicas: 2
      template:
        spec:
          containers:
  -         - image: ghcr.io/shop/api:35bd16b
  +         - image: ghcr.io/shop/api:15df5e1
              imagePullPolicy: IfNotPresent
              name: api
`, b.String())
}

// A create is all added and a delete all removed. A sensitive value is
// never shown, only where it is and whether it changes; a value known only
// once applied says so.
func TestFullDiffMarksCreatesDeletesAndWhatItCannotShow(t *testing.T) {
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{
		{
			Component: "api", Action: agentv1.ResourceChange_ACTION_CREATE, Resource: k8sRef("Secret", "creds"),
			After:          value(t, map[string]any{"kind": "Secret", "data": map[string]any{"password": nil}}),
			SensitivePaths: []*agentv1.SensitivePath{{Path: keyPath("data", "password"), Changed: true}},
		},
		{
			Component: "api", Action: agentv1.ResourceChange_ACTION_DESTROY, Resource: k8sRef("ConfigMap", "old"),
			Before: value(t, map[string]any{"kind": "ConfigMap"}),
		},
		{
			Component: "net", Action: agentv1.ResourceChange_ACTION_UPDATE,
			Resource:     &agentv1.ResourceRef{Ref: &agentv1.ResourceRef_Terraform{Terraform: &agentv1.TerraformResourceRef{Address: "aws_s3_bucket.logs"}}},
			Before:       value(t, map[string]any{"arn": "arn:aws:s3:::logs", "tags": map[string]any{"team": "a"}}),
			After:        value(t, map[string]any{"arn": nil, "tags": map[string]any{"team": "b"}}),
			UnknownPaths: []*agentv1.DiffPath{keyPath("arn")},
		},
	}}
	var b bytes.Buffer
	WriteFullDiff(&b, d, nil)
	out := b.String()
	assert.Contains(t, out, "+ api: v1 Secret podinfo-dev/creds (create)\n  + data:\n  +   password: <sensitive, changed>\n  + kind: Secret\n")
	assert.Contains(t, out, "\n- api: v1 ConfigMap podinfo-dev/old (destroy)\n  - kind: ConfigMap\n")
	assert.Contains(t, out, "  - arn: arn:aws:s3:::logs\n  + arn: <known after apply>\n")
	assert.Contains(t, out, "    tags:\n  -   team: a\n  +   team: b\n")
}

func TestLineDiffKeepsWhatIsCommonInOrder(t *testing.T) {
	assert.Equal(t, []string{"  a", "- b", "+ x", "  c", "+ d"}, lineDiff([]string{"a", "b", "c"}, []string{"a", "x", "c", "d"}))
	assert.Equal(t, []string{"+ a"}, lineDiff(nil, []string{"a"}))
}
