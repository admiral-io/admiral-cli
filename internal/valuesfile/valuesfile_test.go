package valuesfile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What is worth proving: that a file survives download and upload unchanged
// (references, escapes and big numbers included), and that --set reads a
// value the way helm users expect.

func TestParse_RefAndEscape(t *testing.T) {
	tree, warnings, err := Parse([]byte(`
host: !ref users-db.connection_name
literal:
  $ref: not-a-reference
id: 12345678901234567890
ratio: 0.5
on: true
`))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"$ref": "users-db.connection_name"}, tree["host"])
	assert.Equal(t, map[string]any{"$$ref": "not-a-reference"}, tree["literal"])
	assert.Equal(t, json.Number("12345678901234567890"), tree["id"])
	assert.Equal(t, json.Number("0.5"), tree["ratio"])
	assert.Equal(t, true, tree["on"])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "literal: a map whose only key is $ref is stored as $$ref")
}

func TestRender_RoundTrips(t *testing.T) {
	in := map[string]any{
		"host":    map[string]any{"$ref": "users-db.connection_name"},
		"literal": map[string]any{"$$ref": "x"},
		"id":      json.Number("12345678901234567890"),
		"big":     json.Number("1e+21"),
		"tag":     "3",
		"flag":    "true",
		"none":    nil,
		"list":    []any{json.Number("1"), "a"},
	}
	out, err := Render(in, Header{ChangeSet: "cs-abc123def456", Component: "api", Revision: 4, BaseDigest: "sha256:00"})
	require.NoError(t, err)
	assert.Contains(t, string(out), "host: !ref users-db.connection_name\n")
	assert.Contains(t, string(out), "# revision: 4\n")

	back, warnings, err := Parse(out)
	require.NoError(t, err)
	assert.Equal(t, in, back)
	assert.Len(t, warnings, 1, "the unescaped literal is escaped again on the way back, with a warning")

	h, ok := ParseHeader(out)
	require.True(t, ok)
	assert.Equal(t, int32(4), h.Revision)
}

func TestParseScalar_LikeHelm(t *testing.T) {
	testCases := []struct {
		in   string
		want any
	}{
		{in: "true", want: true},
		{in: "3", want: json.Number("3")},
		{in: `"3"`, want: "3"},
		{in: "null", want: nil},
		{in: "sha-4f2a1c", want: "sha-4f2a1c"},
		{in: "0x1F", want: json.Number("31")},
		{in: "!ref users-db.host", want: map[string]any{"$ref": "users-db.host"}},
		{in: "[a, b]", want: []any{"a", "b"}},
		{in: "", want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseScalar(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	_, err := ParseScalar(".inf")
	assert.ErrorContains(t, err, "is not a number JSON can hold")
}

func TestParseAssignment(t *testing.T) {
	a, err := ParseAssignment(`api.ingress.annotations."nginx.ingress.kubernetes.io/rewrite-target"=/`)
	require.NoError(t, err)
	assert.Equal(t, "api", a.Component)
	assert.Equal(t, []string{"ingress", "annotations", "nginx.ingress.kubernetes.io/rewrite-target"}, a.Path)
	assert.Equal(t, "/", a.Raw)

	a, err = ParseAssignment(`api.node\.role=a=b`)
	require.NoError(t, err)
	assert.Equal(t, []string{"node.role"}, a.Path)
	assert.Equal(t, "a=b", a.Raw)

	_, err = ParseAssignment("api=3")
	assert.ErrorContains(t, err, "names no path inside the component")
	_, err = ParseAssignment("api.image.tag")
	assert.ErrorContains(t, err, "is not component.path=value")

	comp, path, err := ParsePath("api.image.tag")
	require.NoError(t, err)
	assert.Equal(t, "api", comp)
	assert.Equal(t, []string{"image", "tag"}, path)
}

func TestParse_RefusesWhatJSONCannotHold(t *testing.T) {
	_, _, err := Parse([]byte("a: !secret x\n"))
	assert.ErrorContains(t, err, "a: tag !secret is not supported")
	_, _, err = Parse([]byte("- a\n"))
	assert.ErrorContains(t, err, "values must be a map at the top level")
}
