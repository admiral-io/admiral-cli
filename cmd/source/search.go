package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

var artifactTable = output.Table[*sourcev1.SourceArtifact]{
	{Header: "NAME", Cell: func(a *sourcev1.SourceArtifact) string { return a.Name }},
	{Header: "LATEST", Cell: func(a *sourcev1.SourceArtifact) string { return a.LatestVersion }},
	{Header: "DESCRIPTION", Truncate: 60, Cell: func(a *sourcev1.SourceArtifact) string { return a.Description }},
}

var versionTable = output.Table[*sourcev1.SourceVersion]{
	{Header: "VERSION", Cell: func(v *sourcev1.SourceVersion) string { return v.Name }},
	{Header: "KIND", Cell: func(v *sourcev1.SourceVersion) string { return output.FormatEnumKebab(v.Kind) }},
	{Header: "COMMIT", Cell: func(v *sourcev1.SourceVersion) string { return short(v.Commit) }},
	{Header: "CREATED", Cell: func(v *sourcev1.SourceVersion) string { return output.FormatAge(v.CreatedAt) }},
}

func newSearchCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions

	cmd := &cobra.Command{
		Use:   "search <source> [artifact]",
		Short: "List what a source offers, or the versions of one artifact",
		Long: `List what a source offers within what it allows, or, with an artifact, that
artifact's versions, newest first. A git repository's versions are its
tags and branches. A container registry and a git repository list no
artifacts: name the artifact.

What is listed is cached for a while and served stale, with a warning,
when the upstream does not answer.`,
		Example: `  # The charts a source offers
  admiral source search argo-helm

  # Argo Rollouts' versions
  admiral source search argo-helm argo-rollouts`,
		Args:              flags.RangeArgs(1, 2),
		ValidArgsFunction: complete.First(complete.Sources(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)

			if len(args) == 1 && src.Kind != sourcev1.Source_KIND_GIT {
				return printArtifacts(cmd, c.Source(), p, src, paging)
			}
			artifact := ""
			if len(args) == 2 {
				artifact = args[1]
			}
			return printVersions(cmd, c.Source(), p, src, artifact, paging)
		},
	}

	flags.Paging(cmd, &paging)
	return cmd
}

func warnStale(p *output.Printer, reason string) {
	if reason != "" {
		output.Writef(p.Err(), "Warning: the upstream did not answer, so this is the last list that succeeded: %s\n", reason)
	}
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func printArtifacts(cmd *cobra.Command, c sourcev1.SourceAPIClient, p *output.Printer, src *sourcev1.Source, paging flags.PagingOptions) error {
	var stale string
	artifacts, next, err := flags.Pages(paging, func(token string) ([]*sourcev1.SourceArtifact, string, error) {
		resp, err := c.ListSourceArtifacts(cmd.Context(), &sourcev1.ListSourceArtifactsRequest{
			SourceId: src.Id, PageSize: paging.PageSize, PageToken: token,
		})
		if err != nil {
			return nil, "", err
		}
		stale = resp.StaleReason
		return resp.Artifacts, resp.NextPageToken, nil
	})
	if err != nil {
		return err
	}
	warnStale(p, stale)
	return p.PrintList(output.List{
		Kind: "artifacts", Items: output.Messages(artifacts),
		Name:          func(i int) string { return artifacts[i].Name },
		NextPageToken: next,
	}, artifactTable.Render(p, artifacts...))
}

func printVersions(cmd *cobra.Command, c sourcev1.SourceAPIClient, p *output.Printer, src *sourcev1.Source, artifact string, paging flags.PagingOptions) error {
	var stale string
	versions, next, err := flags.Pages(paging, func(token string) ([]*sourcev1.SourceVersion, string, error) {
		resp, err := c.ListSourceVersions(cmd.Context(), &sourcev1.ListSourceVersionsRequest{
			SourceId: src.Id, Artifact: artifact, PageSize: paging.PageSize, PageToken: token,
		})
		if err != nil {
			return nil, "", err
		}
		stale = resp.StaleReason
		return resp.Versions, resp.NextPageToken, nil
	})
	if err != nil {
		return err
	}
	warnStale(p, stale)
	return p.PrintList(output.List{
		Kind: "versions", Items: output.Messages(versions),
		Name:          func(i int) string { return versions[i].Name },
		NextPageToken: next,
	}, versionTable.Render(p, versions...))
}
