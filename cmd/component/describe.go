package component

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

const (
	// versionsShown is how many versions, newest first, the table lists.
	versionsShown = 10
	// newerShown is how many newer upstream versions are named.
	newerShown = 5
	// versionsRead caps the versions read to find the newest imported.
	versionsRead = 500
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Show a component, where its versions come from, and where each runs",
		Long: `Render the operator view of a component: its origin, its versions newest
first with the environments running each, and, for a component imported
through a source, the versions the source lists that are newer than any
imported. A section the active credential cannot read is shown as
unavailable.

describe is a human view. Use 'component get -o json' for the raw record.`,
		Example: `  admiral component describe argo-rollouts
  admiral component describe cmp-7q2k9m4x8p1c`,
		Aliases:           []string{"desc"},
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Registry().GetComponent(cmd.Context(), &registryv1.GetComponentRequest{Component: args[0]})
			if err != nil {
				return err
			}

			sec := readComponentSections(cmd.Context(), c, resp.Component)
			p := output.NewPrinter(cmd, opts.OutputFormat)

			return p.PrintDescribe(describeComponent(resp.Component, sec), "admiral component get "+resp.Component.Name)
		},
	}

	return cmd
}

// componentSections holds the reads describe assembles around the
// component. Each read carries its own error so a failed section renders
// as unavailable without hiding the others.
type componentSections struct {
	source *sourcev1.Source

	versions    []*registryv1.Version
	versionsErr error

	// runs is the environments running each version, by version ID, as
	// project/environment paths.
	runs           map[string][]string
	runsErr        error
	unreadableEnvs int32

	upstream    []*sourcev1.SourceVersion
	upstreamErr error
}

func readComponentSections(ctx context.Context, c sdkclient.AdmiralClient, comp *registryv1.Component) componentSections {
	var sec componentSections

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		sec.versions, sec.versionsErr = componentVersions(gctx, c.Registry(), comp.Id)
		return nil
	})
	g.Go(func() error {
		sec.runs, sec.unreadableEnvs, sec.runsErr = runningVersions(gctx, c, comp.Id)
		return nil
	})

	if o := comp.GetOrigin().GetSource(); o != nil {
		g.Go(func() error {
			sec.source, sec.upstream, sec.upstreamErr = upstreamVersions(gctx, c.Source(), o)
			return nil
		})
	}

	_ = g.Wait() // nothing returns an error

	return sec
}

// componentVersions is the component's versions as the registry lists
// them, newest first, up to versionsRead.
func componentVersions(ctx context.Context, c registryv1.RegistryAPIClient, componentID string) ([]*registryv1.Version, error) {
	var out []*registryv1.Version

	token := ""
	for len(out) < versionsRead {
		resp, err := c.ListVersions(ctx, &registryv1.ListVersionsRequest{ComponentId: componentID, PageSize: 100, PageToken: token})
		if err != nil {
			return nil, err
		}

		out = append(out, resp.Versions...)
		if resp.NextPageToken == "" {
			break
		}

		token = resp.NextPageToken
	}

	return out, nil
}

// runningVersions is every environment running the component, by the
// version it runs, and how many environments the caller cannot read.
func runningVersions(ctx context.Context, c sdkclient.AdmiralClient, componentID string) (map[string][]string, int32, error) {
	var (
		live       []*environmentv1.EnvironmentComponent
		unreadable int32
	)

	token := ""
	for {
		resp, err := c.Environment().ListEnvironmentComponents(ctx, &environmentv1.ListEnvironmentComponentsRequest{
			Filter: fmt.Sprintf("field['component_id'] = '%s'", componentID), PageSize: 100, PageToken: token,
		})
		if err != nil {
			return nil, 0, err
		}

		live = append(live, resp.Components...)

		unreadable = resp.UnreadableEnvironments
		if resp.NextPageToken == "" {
			break
		}

		token = resp.NextPageToken
	}

	out := map[string][]string{}
	if len(live) == 0 {
		return out, unreadable, nil
	}

	paths := environmentPaths(ctx, c)
	for _, ec := range live {
		path := paths[ec.EnvironmentId]
		if path == "" {
			path = ec.EnvironmentId
		}

		id := ec.GetVersion().GetVersionId()
		out[id] = append(out[id], path)
	}

	for id := range out {
		slices.Sort(out[id])
	}

	return out, unreadable, nil
}

// environmentPaths names every environment the caller can read as
// project/environment. A failed read leaves the IDs.
func environmentPaths(ctx context.Context, c sdkclient.AdmiralClient) map[string]string {
	projects := map[string]string{}

	token := ""
	for {
		resp, err := c.Project().ListProjects(ctx, &projectv1.ListProjectsRequest{PageSize: 100, PageToken: token})
		if err != nil {
			break
		}

		for _, p := range resp.Projects {
			projects[p.Id] = p.Name
		}

		if token = resp.NextPageToken; token == "" {
			break
		}
	}

	paths := map[string]string{}

	token = ""
	for {
		resp, err := c.Environment().ListEnvironments(ctx, &environmentv1.ListEnvironmentsRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return paths
		}

		for _, e := range resp.Environments {
			project := projects[e.ProjectId]
			if project == "" {
				project = e.ProjectId
			}

			paths[e.Id] = project + "/" + e.Name
		}

		if token = resp.NextPageToken; token == "" {
			return paths
		}
	}
}

// upstreamVersions is the origin's source and what it lists for the
// artifact now. The source is nil when it could not be read.
func upstreamVersions(ctx context.Context, c sourcev1.SourceAPIClient, o *registryv1.SourceOrigin) (*sourcev1.Source, []*sourcev1.SourceVersion, error) {
	resp, err := c.GetSource(ctx, &sourcev1.GetSourceRequest{SourceId: o.SourceId})
	if err != nil {
		return nil, nil, err
	}

	src := resp.Source

	artifact := o.Artifact
	if src.Kind == sourcev1.Source_KIND_GIT {
		artifact = "" // a repository's tags do not depend on the path
	}

	var out []*sourcev1.SourceVersion

	token := ""
	for {
		vs, err := c.ListSourceVersions(ctx, &sourcev1.ListSourceVersionsRequest{
			SourceId: src.Id, Artifact: artifact, PageSize: 200, PageToken: token,
		})
		if err != nil {
			return src, nil, err
		}

		out = append(out, vs.Versions...)
		if token = vs.NextPageToken; token == "" {
			return src, out, nil
		}
	}
}

// newerUpstream is what the source lists that is a released version
// higher than every version imported, highest first. Branches,
// pre-releases and names that are not semver are left out, as latest
// leaves them out.
func newerUpstream(upstream []*sourcev1.SourceVersion, imported []*registryv1.Version) []string {
	newest := ""
	for _, v := range imported {
		if sv := canonical(v.GetProvenance().GetRef()); sv != "" && (newest == "" || semver.Compare(sv, newest) > 0) {
			newest = sv
		}
	}

	type candidate struct{ name, sv string }

	var newer []candidate

	for _, v := range upstream {
		if v.Kind != sourcev1.SourceVersion_KIND_VERSION {
			continue
		}

		sv := canonical(v.Name)
		if sv == "" || semver.Prerelease(sv) != "" || (newest != "" && semver.Compare(sv, newest) <= 0) {
			continue
		}

		newer = append(newer, candidate{v.Name, sv})
	}

	slices.SortFunc(newer, func(a, b candidate) int { return semver.Compare(b.sv, a.sv) })

	out := make([]string, 0, len(newer))
	for _, c := range newer {
		out = append(out, c.name)
	}

	return out
}

// canonical is a version name as golang.org/x/mod/semver reads it, with
// its v, or empty when it is not semver.
func canonical(name string) string {
	if name == "" {
		return ""
	}

	if !strings.HasPrefix(name, "v") {
		name = "v" + name
	}

	if !semver.IsValid(name) {
		return ""
	}

	return name
}

// versionName is how a version is spoken of: the upstream version it was
// imported as, else its tags, else its short digest.
func versionName(v *registryv1.Version) string {
	if ref := v.GetProvenance().GetRef(); ref != "" && v.GetProvenance().GetKind() == registryv1.Provenance_KIND_IMPORT {
		return ref
	}

	if len(v.Tags) > 0 {
		return strings.Join(v.Tags, ",")
	}

	return shortDigest(v.Digest)
}

func describeComponent(comp *registryv1.Component, sec componentSections) *output.Describe {
	d := output.NewDescribe()
	d.Field("Name", comp.Name)
	d.Field("ID", comp.Id)
	d.Field("Description", comp.Description)
	d.Fields("Labels", output.LabelLines(comp.Labels))

	names := map[string]string{}
	if sec.source != nil {
		names[sec.source.Id] = sec.source.Name
	}

	d.Field("Origin", originLabel(comp.Origin, names))

	status := output.FormatEnumKebab(comp.Status)
	if comp.StatusReason != "" {
		status += ": " + comp.StatusReason
	}

	d.Field("Status", status)
	d.Field("Tags", formatTags(comp.Tags))
	d.Field("Created", output.FormatDescribeTime(comp.CreatedAt))
	d.Field("Created By", output.FormatActorWithID(comp.CreatedBy))

	describeVersions(d, sec)
	describeNewer(d, comp, sec)

	return d
}

func describeVersions(d *output.Describe, sec componentSections) {
	if sec.versionsErr != nil {
		d.Unavailable("Versions", cmderr.Format(sec.versionsErr))
		return
	}

	d.Section("Versions", func(b *output.Block) {
		if len(sec.versions) == 0 {
			b.Field("Count", "0")
			return
		}

		shown := sec.versions[:min(len(sec.versions), versionsShown)]

		rows := make([][]string, 0, len(shown))
		for _, v := range shown {
			runs := strings.Join(sec.runs[v.Id], ", ")
			if sec.runsErr != nil {
				runs = "<unavailable>"
			}

			rows = append(rows, []string{versionName(v), shortDigest(v.Digest), output.FormatEnumKebab(v.Status), output.FormatAge(v.CreatedAt), runs})
		}

		b.Table([]string{"Version", "Digest", "Status", "Age", "Runs In"}, rows)

		if more := len(sec.versions) - len(shown); more > 0 {
			b.Line(fmt.Sprintf("+%d older; 'admiral component version list' lists them all", more))
		}

		if sec.runsErr != nil {
			b.Field("Runs In", "<unavailable: "+cmderr.Format(sec.runsErr)+">")
		} else if sec.unreadableEnvs > 0 {
			b.Line(fmt.Sprintf("%d more environment(s) you cannot read may run it", sec.unreadableEnvs))
		}
	})
}

func describeNewer(d *output.Describe, comp *registryv1.Component, sec componentSections) {
	if comp.GetOrigin().GetSource() == nil {
		return
	}

	title := "Newer Upstream"
	if sec.source != nil {
		title = "Newer In " + sec.source.Name
	}

	if err := firstErr(sec.upstreamErr, sec.versionsErr); err != nil {
		d.Unavailable(title, cmderr.Format(err))
		return
	}

	newer := newerUpstream(sec.upstream, sec.versions)

	d.Section(title, func(b *output.Block) {
		if len(newer) == 0 {
			b.Line("none; the newest imported is the newest released")
			return
		}

		line := strings.Join(newer[:min(len(newer), newerShown)], ", ")
		if more := len(newer) - newerShown; more > 0 {
			line += fmt.Sprintf(", +%d", more)
		}

		b.Line(line)
	})

	if len(newer) > 0 {
		d.Hint(fmt.Sprintf("To import the newest, run: admiral component import %s --version %s", comp.Name, newer[0]))
	}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}
