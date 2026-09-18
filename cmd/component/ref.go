package component

import (
	"strconv"
	"strings"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// splitRef takes a reference apart: NAME:TAG, NAME@DIGEST, or a bare NAME.
// The digest keeps its sha256: prefix, which is how the API tells it from a
// tag. The empty reference is the caller's to refuse or default.
func splitRef(s string) (name, ref string, err error) {
	if i := strings.Index(s, "@"); i >= 0 {
		name, ref = s[:i], s[i+1:]
	} else if i := strings.Index(s, ":"); i >= 0 {
		name, ref = s[:i], s[i+1:]
	} else {
		name = s
	}
	if name == "" {
		return "", "", cmderr.Usage("reference %q has no component name", s)
	}
	if (strings.Contains(s, "@") || strings.Contains(s, ":")) && ref == "" {
		return "", "", cmderr.Usage("reference %q names no tag or digest", s)
	}
	return name, ref, nil
}

// tagTable is what a tag answers with.
var tagTable = output.Table[*registryv1.Tag]{
	{Header: "NAME", Cell: func(t *registryv1.Tag) string { return t.Name }},
	{Header: "DIGEST", Cell: func(t *registryv1.Tag) string { return shortDigest(t.Digest) }},
	{Header: "IMMUTABLE", Cell: func(t *registryv1.Tag) string { return strconv.FormatBool(t.Immutable) }},
	{Header: "AGE", Cell: func(t *registryv1.Tag) string { return output.FormatAge(t.CreatedAt) }},
	{Header: "FULL-DIGEST", Wide: true, Cell: func(t *registryv1.Tag) string { return t.Digest }},
}
