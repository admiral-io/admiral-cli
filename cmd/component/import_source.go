package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// through is an import through one of the organization's sources, as the
// request carries it.
type through struct {
	sourceID, artifact, version string
	label                       string
}

// throughSource resolves the source and fills what the import leaves out:
// the artifact defaults to the component's name, except in a git
// repository, where empty is the root. A missing version is asked for in a
// terminal, from the source's list, with nothing pre-selected; without a
// terminal the list is in the refusal. The version is never guessed, so a
// branch is imported only when someone picked it over the tags listed with
// it.
func throughSource(cmd *cobra.Command, c sourcev1.SourceAPIClient, nameOrID, name, artifact, version string) (through, error) {
	ctx := cmd.Context()

	src, err := resolve.Source(ctx, c, nameOrID)
	if err != nil {
		return through{}, err
	}

	if src.Status != sourcev1.Source_STATUS_READY {
		return through{}, cmderr.WithHint(
			fmt.Errorf("source %s needs attention: %s", src.Name, src.StatusReason),
			"Run 'admiral source get "+src.Name+"' to see why, then import again.")
	}

	artifact, err = artifactIn(src, name, artifact)
	if err != nil {
		return through{}, err
	}

	if version == "" {
		// A repository's tags and branches do not depend on the path, and
		// asking without one reads the same cached answer the console does.
		versionsArtifact := artifact
		if src.Kind == sourcev1.Source_KIND_GIT {
			versionsArtifact = ""
		}

		options, err := versionsOf(ctx, c, src, versionsArtifact)
		if err != nil {
			return through{}, err
		}

		picked, err := input.Choose(cmd, "--version", "version", options)
		if err != nil {
			return through{}, err
		}

		version = picked.Name
	}

	what := artifact
	if what == "" {
		what = src.Location
	}

	return through{
		sourceID: src.Id, artifact: artifact, version: version,
		label: fmt.Sprintf("%s %s from %s", what, version, src.Name),
	}, nil
}

// artifactIn is the artifact as the request carries it: what was given,
// or the component's name everywhere but a git repository, whose root is
// empty.
func artifactIn(src *sourcev1.Source, name, artifact string) (string, error) {
	artifact = strings.Trim(strings.TrimSpace(artifact), "/")

	switch src.Kind {
	case sourcev1.Source_KIND_GIT:
		return artifact, nil
	case sourcev1.Source_KIND_HELM_REPO, sourcev1.Source_KIND_OCI_REGISTRY, sourcev1.Source_KIND_TERRAFORM_REGISTRY:
		if artifact == "" {
			return name, nil
		}

		return artifact, nil
	default:
		return "", fmt.Errorf("source %s is of a kind this command cannot import from", src.Name)
	}
}

// originOf is the source and artifact a bump goes through, the ones the
// component's origin names. A name that is not a component yet needs
// --source, and a component published by hand is bumped by publishing.
func originOf(ctx context.Context, c registryv1.RegistryAPIClient, name string) (sourceID, artifact string, err error) {
	resp, err := c.GetComponent(ctx, &registryv1.GetComponentRequest{Component: name})
	if status.Code(err) == codes.NotFound {
		return "", "", cmderr.Usage("%s is not a component yet; name the source to import it from with --source", name)
	}

	if err != nil {
		return "", "", fmt.Errorf("looking up component %q: %w", name, err)
	}

	moveHint := "An organization admin can move it with 'admiral component set-origin " + name + " --source SOURCE'."

	switch o := resp.Component.GetOrigin().GetKind().(type) {
	case *registryv1.ComponentOrigin_Source:
		return o.Source.SourceId, o.Source.Artifact, nil
	case *registryv1.ComponentOrigin_Published:
		return "", "", cmderr.WithHint(
			fmt.Errorf("%s is published by hand, so it is not imported through a source", name),
			"Publish its next version with 'admiral component publish'. "+moveHint)
	default:
		return "", "", cmderr.WithHint(fmt.Errorf("%s has no origin yet", name),
			"Name the source with --source. "+moveHint)
	}
}

// versionsOf is what the source lists for the artifact, in the order the
// source lists it, each said to be a version or a branch.
func versionsOf(ctx context.Context, c sourcev1.SourceAPIClient, src *sourcev1.Source, artifact string) ([]input.Option, error) {
	var out []input.Option

	token := ""
	for {
		resp, err := c.ListSourceVersions(ctx, &sourcev1.ListSourceVersionsRequest{
			SourceId: src.Id, Artifact: artifact, PageSize: 200, PageToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("listing versions of %s: %w", artifact, err)
		}

		for _, v := range resp.Versions {
			detail := "version"
			if v.Kind == sourcev1.SourceVersion_KIND_BRANCH {
				detail = "branch"
			}

			if v.Commit != "" {
				detail += " " + shortCommit(v.Commit)
			}

			out = append(out, input.Option{Name: v.Name, Detail: detail})
		}

		if resp.NextPageToken == "" {
			return out, nil
		}

		token = resp.NextPageToken
	}
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}

	return c
}
