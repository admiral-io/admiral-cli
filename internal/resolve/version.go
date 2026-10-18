package resolve

import (
	"context"
	"strings"

	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// VersionTags reads the tags of each version that is referenced by digest
// or not at all, keyed by version ID, so it can be shown by a tag. A
// version that cannot be read is left out and keeps its digest.
func VersionTags(ctx context.Context, c registryv1.RegistryAPIClient, refs []*changesetv1.VersionRef) map[string][]string {
	tags := map[string][]string{}
	for _, v := range refs {
		if v == nil || v.ComponentId == "" || v.VersionId == "" || isTag(v.Reference) {
			continue
		}
		if _, done := tags[v.VersionId]; done {
			continue
		}
		resp, err := c.GetVersion(ctx, &registryv1.GetVersionRequest{ComponentId: v.ComponentId, Reference: v.VersionId})
		if err != nil {
			continue
		}
		tags[v.VersionId] = resp.GetVersion().GetTags()
	}
	return tags
}

// VersionLabel names a version by the tag it was referenced by, or else
// one of its tags, with its short digest: "6.8.0 (sha256:3902c9ddbe75)".
func VersionLabel(v *changesetv1.VersionRef, tags map[string][]string) string {
	if v == nil {
		return ""
	}
	hex := strings.TrimPrefix(v.Digest, "sha256:")
	short := "sha256:" + hex[:min(len(hex), 12)]
	tag := v.Reference
	if !isTag(tag) {
		tag = ""
		if ts := tags[v.VersionId]; len(ts) > 0 {
			tag = ts[0]
		}
	}
	if tag == "" {
		return short
	}
	return tag + " (" + short + ")"
}

func isTag(ref string) bool {
	return ref != "" && !strings.HasPrefix(ref, "sha256:")
}
