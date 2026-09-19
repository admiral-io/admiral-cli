package component

import (
	"context"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/bundle"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// Tags a publish applies after the bytes are in, each set on its own so
// that one refused tag never takes the publish down with it.
//
// A commit tag (sha-<short>) is set only when this publish created the
// revision: it then reads as "the commits that produced this", not as
// every commit that happened to see it.
//
// A chart's own version (Chart.yaml: version) is its semver event, the way
// chart-releaser treats it, and is applied as an immutable tag. Leniently:
// when the version already names different bytes, because the chart changed
// and nobody bumped it, the publish stands under its floating tags and the
// refusal is printed as a nudge rather than a failure.

// semverPattern is the shape the registry treats as immutable.
var semverPattern = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

type afterPublish struct {
	// sha is the commit tag to apply when the revision is new; "" for none.
	sha string
}

// applyTags sets the tags a publish earned after the fact and returns the
// revision's tag list with the ones that took appended, so a table shows
// what the registry now says.
func applyTags(ctx context.Context, p *output.Printer, c sdkclient.AdmiralClient, resp *registryv1.PublishComponentResponse, packed *bundle.Packed, after afterPublish) []string {
	tags := append([]string(nil), resp.Revision.Tags...)
	comp, rev := resp.Component, resp.Revision

	if after.sha != "" && !resp.Unchanged && !contains(tags, after.sha) {
		if _, err := c.Registry().SetTag(ctx, &registryv1.SetTagRequest{ComponentId: comp.Id, Name: after.sha, Digest: rev.Digest}); err != nil {
			output.Writef(p.Err(), "%s: could not tag %s: %v\n", comp.Name, after.sha, err)
		} else {
			tags = append(tags, after.sha)
		}
	}

	if v := packed.Version; v != "" && semverPattern.MatchString(v) && !contains(tags, v) {
		_, err := c.Registry().SetTag(ctx, &registryv1.SetTagRequest{ComponentId: comp.Id, Name: v, Digest: rev.Digest})
		switch {
		case err == nil:
			tags = append(tags, v)
		case status.Code(err) == codes.FailedPrecondition:
			// The version already names other bytes: the chart changed and its
			// version did not. The registry keeps the version honest; the
			// publish keeps its floating tags.
			output.Writef(p.Err(), "  warn chart version %s already names a different revision; bump version in Chart.yaml to tag this one\n", v)
		default:
			output.Writef(p.Err(), "%s: could not tag %s: %v\n", comp.Name, v, err)
		}
	}
	return tags
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
