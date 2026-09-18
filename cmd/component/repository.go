package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/manifest"
	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// Publishing a repository is the Docker shape: pack every component the
// manifest declares and push each one; the registry is content-addressed,
// so bytes it already holds are a no-op and only what changed becomes a new
// revision. No git diff decides what to send. A shared module's change
// shows up as a new digest for every component that vendors it, because
// their bytes changed, and nothing pinned to a digest moves. The tags
// (<branch> and sha-<short> unless --tag says otherwise) go on every
// declared component, the way pushing an image tags it whether or not a
// layer was new.

type repositoryOptions struct {
	manifestPath string
	tags         []string
	labels       map[string]string
	description  string
}

func publishRepository(cmd *cobra.Command, opts *client.Options, o repositoryOptions) error {
	ctx := cmd.Context()
	p := output.NewPrinter(cmd, opts.OutputFormat)

	m, err := manifest.Read(o.manifestPath)
	if err != nil {
		return err
	}
	root := filepath.Dir(o.manifestPath)

	tags := o.tags
	if len(tags) == 0 {
		tags = defaultTags(ctx, root)
	}

	cl, err := client.CreateClient(ctx, opts)
	if err != nil {
		return err
	}
	defer cl.Close() //nolint:errcheck

	var (
		revisions []*registryv1.Revision
		rows      []publishedRow
		names     []string
		failures  []string
	)
	for _, c := range m.Components {
		abs := filepath.Join(root, filepath.FromSlash(c.Path))
		packed, err := pack(p, abs, c.Path)
		if err != nil {
			output.Writef(p.Err(), "%s: %v\n", c.Name, err)
			failures = append(failures, c.Name)
			continue
		}
		resp, err := publishOne(cmd, p, cl, publishRequest{
			name: c.Name, tags: tags, description: o.description, labels: o.labels,
			dir: abs, packed: packed,
		})
		if err != nil {
			output.Writef(p.Err(), "%s: %v\n", c.Name, err)
			failures = append(failures, c.Name)
			continue
		}
		revisions = append(revisions, resp.Revision)
		rows = append(rows, publishedRow{name: resp.Component.Name, rev: resp.Revision, unchanged: resp.Unchanged})
		names = append(names, resp.Component.Name+"@"+resp.Revision.Digest)
	}

	if err := p.PrintList(output.List{
		Kind:  "revisions",
		Items: output.Messages(revisions),
		Name:  func(i int) string { return names[i] },
	}, publishedTable.Render(p, rows...)); err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d components failed to publish: %s", len(failures), len(m.Components), strings.Join(failures, ", "))
	}
	return nil
}

// defaultTags is the push policy: the branch, and sha-<short>. On a detached
// HEAD (which is what CI checks out) the branch comes from GITHUB_REF_NAME
// when set. A ref with a slash in it is not a tag name and is left out.
// Outside git, nothing is applied and the caller's own --tag is the way.
func defaultTags(ctx context.Context, root string) []string {
	var tags []string
	branch, _ := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" || branch == "" {
		branch = os.Getenv("GITHUB_REF_NAME")
	}
	if branch != "" && !strings.Contains(branch, "/") {
		tags = append(tags, branch)
	}
	if short, err := gitOutput(ctx, root, "rev-parse", "--short=7", "HEAD"); err == nil && short != "" {
		tags = append(tags, "sha-"+short)
	}
	return tags
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// publishedRow is one component's outcome in a repository publish: the
// table needs the name, which the revision does not carry.
type publishedRow struct {
	name      string
	rev       *registryv1.Revision
	unchanged bool
}

var publishedTable = output.Table[publishedRow]{
	{Header: "COMPONENT", Cell: func(r publishedRow) string { return r.name }},
	{Header: "DIGEST", Cell: func(r publishedRow) string { return shortDigest(r.rev.Digest) }},
	{Header: "TAGS", Cell: func(r publishedRow) string { return strings.Join(r.rev.Tags, ",") }},
	{Header: "RESULT", Cell: func(r publishedRow) string {
		if r.unchanged {
			return "unchanged"
		}
		return "published"
	}},
	{Header: "FINDINGS", Cell: func(r publishedRow) string { return formatFindingCount(r.rev.Findings) }},
	{Header: "FULL-DIGEST", Wide: true, Cell: func(r publishedRow) string { return r.rev.Digest }},
}
