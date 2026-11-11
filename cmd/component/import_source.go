package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// fromSource builds an import through one of the organization's sources:
// the artifact as the source lists it, at a version spelled exactly as the
// source lists that. What is not given is asked for in a terminal, from
// the list, with nothing pre-selected; without a terminal the list is in
// the refusal. The version is never guessed, so a branch is imported only
// when someone picked it over the tags listed with it, and `latest` is a
// tag literally named latest.
func fromSource(cmd *cobra.Command, c sourcev1.SourceAPIClient, from, artifact, version string) (*registryv1.ImportSource, string, error) {
	ctx := cmd.Context()
	src, err := resolve.Source(ctx, c, from)
	if err != nil {
		return nil, "", err
	}
	if src.Status != sourcev1.Source_STATUS_READY {
		return nil, "", cmderr.WithHint(
			fmt.Errorf("source %s needs attention: %s", src.Name, src.StatusReason),
			"Fix the source under Sources in the console, then import again.")
	}

	artifact = strings.Trim(strings.TrimSpace(artifact), "/")
	switch src.Kind {
	case sourcev1.Source_KIND_GIT:
		// A path in the repository; empty is the root.
	case sourcev1.Source_KIND_OCI_REGISTRY:
		if artifact == "" {
			return nil, "", cmderr.Usage("a container registry lists nothing: name the artifact under %s", src.Location)
		}
	case sourcev1.Source_KIND_HELM_REPO, sourcev1.Source_KIND_TERRAFORM_REGISTRY:
		if artifact == "" {
			options, err := artifactsOf(ctx, c, src)
			if err != nil {
				return nil, "", err
			}
			picked, err := input.Choose(cmd, "ARTIFACT", "artifact", options)
			if err != nil {
				return nil, "", err
			}
			artifact = picked.Name
		}
	default:
		return nil, "", fmt.Errorf("source %s is of a kind this command cannot import from", src.Name)
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
			return nil, "", err
		}
		picked, err := input.Choose(cmd, "--version", "version", options)
		if err != nil {
			return nil, "", err
		}
		version = picked.Name
	}

	what := artifact
	if what == "" {
		what = src.Location
	}
	return &registryv1.ImportSource{Source: &registryv1.ImportSource_FromSource_{FromSource: &registryv1.ImportSource_FromSource{
		SourceId: src.Id, Artifact: artifact, Version: version,
	}}}, fmt.Sprintf("%s %s from %s", what, version, src.Name), nil
}

// artifactsOf is what the source offers within what it allows, as the
// platform's cache serves it.
func artifactsOf(ctx context.Context, c sourcev1.SourceAPIClient, src *sourcev1.Source) ([]input.Option, error) {
	var out []input.Option
	token := ""
	for {
		resp, err := c.ListSourceArtifacts(ctx, &sourcev1.ListSourceArtifactsRequest{
			SourceId: src.Id, PageSize: 200, PageToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("listing what %s offers: %w", src.Name, err)
		}
		for _, a := range resp.Artifacts {
			detail := a.Description
			if a.LatestVersion != "" {
				detail = "latest " + a.LatestVersion
			}
			out = append(out, input.Option{Name: a.Name, Detail: detail})
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		token = resp.NextPageToken
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
