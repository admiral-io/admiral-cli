package component

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/bundle"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/manifest"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// maxBundleBytes is the registry's cap on an inline bundle. Checked here so
// the answer is a sentence, not a rejected upload after the upload.
const maxBundleBytes = 64 << 20

var namePattern = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

func newPublishCmd(opts *client.Options) *cobra.Command {
	var (
		name        string
		kind        string
		tags        []string
		description string
		labelStrs   []string
		manifestArg string
	)

	cmd := &cobra.Command{
		Use:   "publish [path]",
		Short: "Publish a directory as a component revision",
		Long: `Publish a directory as a component revision.

The directory is packed and uploaded; the registry normalizes it, digests
it, inspects it for its contract, and stores it. Publishing the same tree
twice yields the same revision, so a publish-everything CI step is safe.

Terraform modules are closed before upload: every module call whose source
is a local path outside the directory is copied into vendor/ and the call
rewritten to point there, recursively. The working copy is never modified.
Remote sources (registry addresses, git URLs) are not fetched yet; vendor
them into the tree first.

The component is created on first publish, named after the directory
unless --name says otherwise. --tag names the revision; a semver tag can be
set once and never moved.

Pointed at a directory that holds admiral.yaml, publish takes the whole
repository: every component the manifest declares is packed and pushed. The
registry is content-addressed, so a component whose bytes it already holds
is a no-op and only what changed becomes a new revision; a shared module's
change shows up in every component that vendors it. Without --tag the
revisions are tagged with the branch name and sha-<short>, on every declared
component. This is the form CI runs on every push.`,
		Example: `  # Publish the current directory, named after it
  admiral component publish

  # Publish a module under a version and as latest
  admiral component publish ./modules/cloud-sql --tag v1.2.0 --tag latest

  # A dev build under a floating tag, with a different name
  admiral component publish . --name cloud-sql-dev --tag dev

  # The repository: everything admiral.yaml declares, tagged <branch> and
  # sha-<short>; what the registry already has is a no-op
  admiral component publish

  # A manifest that is not at the current directory
  admiral component publish -f infra/admiral.yaml`,
		Args: flags.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}

			// The thing pointed at says what it is: a manifest, or a
			// directory holding one, is a repository; anything else is one
			// component, and Detect decides its kind.
			manifestPath := manifestArg
			if manifestPath == "" && manifest.Exists(abs) {
				manifestPath = filepath.Join(abs, manifest.Filename)
			}
			if manifestPath != "" {
				if name != "" || kind != "" {
					return cmderr.Usage("a repository publish takes its names and kinds from %s; --name and --kind apply to one component", manifest.Filename)
				}
				if manifestArg != "" && len(args) == 1 {
					return cmderr.Usage("pass either a directory or --manifest, not both")
				}
				manifestPath, err = filepath.Abs(manifestPath)
				if err != nil {
					return err
				}
				return publishRepository(cmd, opts, repositoryOptions{
					manifestPath: manifestPath, tags: tags, labels: labels, description: description,
				})
			}
			if name == "" {
				name = filepath.Base(abs)
			}
			if !namePattern.MatchString(name) {
				return cmderr.Usage("component name %q must be lowercase letters, digits and hyphens (use --name)", name)
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			packed, err := pack(cmd.Context(), p, abs, dir)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := publishOne(cmd, p, c, publishRequest{
				name: name, kind: kindEnum(kind), tags: tags, description: description, labels: labels,
				dir: abs, packed: packed,
			})
			if err != nil {
				return err
			}
			rev := resp.Revision
			rev.Tags = applyTags(cmd.Context(), p, c, resp, packed, afterPublish{})
			return p.PrintOne(rev, resp.Component.Name+"@"+rev.Digest, revisionTable.Render(p, rev))
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "component name (default: the directory's name)")
	flags.Enum(cmd, &kind, "kind", "", "what the directory is, when detection should not decide", "terraform", "helm", "manifests")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "tag to set on the revision (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "component description, applied when the component is created")
	flags.Label(cmd, &labelStrs, "label to attach when the component is created (key=value, repeatable)")
	cmd.Flags().StringVarP(&manifestArg, "manifest", "f", "", "publish the repository this "+manifest.Filename+" describes (default: the one in the directory, if any)")

	return cmd
}

// pack stages and packs one component directory, reporting what it vendored.
// display is the path as the user typed it, for the messages.
func pack(ctx context.Context, p *output.Printer, abs, display string) (*bundle.Packed, error) {
	packed, err := bundle.PackContext(ctx, abs, bundle.NewAmbientCredentials())
	if err != nil {
		return nil, err
	}
	for _, v := range packed.Vendored {
		output.Writef(p.Err(), "Vendored %s (from %s) into %s\n", v.Source, displayCaller(v.Caller), v.Into)
	}
	for _, pin := range packed.Pins {
		if pin.Constraint != "" {
			output.Writef(p.Err(), "Pinned %s %s to %s\n", pin.Source, pin.Constraint, pin.Resolved)
		} else {
			output.Writef(p.Err(), "Pinned %s to %s\n", pin.Source, pin.Resolved)
		}
	}
	if len(packed.Bytes) > maxBundleBytes {
		return nil, fmt.Errorf("bundle is %s, over the %s limit", formatSize(int64(len(packed.Bytes))), formatSize(maxBundleBytes))
	}
	output.Writef(p.Err(), "Packed %s: %d files, %s, %s\n", display, packed.Files, formatSize(int64(len(packed.Bytes))), strings.ToLower(string(packed.Kind)))
	return packed, nil
}

type publishRequest struct {
	name        string
	kind        registryv1.ComponentKind
	tags        []string
	description string
	labels      map[string]string
	dir         string
	packed      *bundle.Packed
}

// publishOne uploads a packed component and reports the outcome and the
// findings on stderr. The caller decides how to print the revision.
func publishOne(cmd *cobra.Command, p *output.Printer, c sdkclient.AdmiralClient, req publishRequest) (*registryv1.PublishComponentResponse, error) {
	prov := bundle.Describe(cmd.Context(), req.dir)
	if prov.Dirty {
		output.Writef(p.Err(), "Working copy has uncommitted changes; the revision will say so\n")
	}
	resp, err := c.Registry().PublishComponent(cmd.Context(), &registryv1.PublishComponentRequest{
		Name:        req.name,
		Kind:        req.kind,
		Tags:        req.tags,
		Description: req.description,
		Labels:      req.labels,
		Provenance: &registryv1.Provenance{
			Uri: prov.URI, Commit: prov.Commit, Path: prov.Path, Dirty: prov.Dirty,
			Pins: pinsToProto(req.packed.Pins),
		},
		Bundle: req.packed.Bytes,
	})
	if err != nil {
		return nil, err
	}
	rev := resp.Revision
	if resp.Unchanged {
		output.Writef(p.Err(), "%s already published as %s; nothing written\n", resp.Component.Name, shortDigest(rev.Digest))
	} else {
		output.Writef(p.Err(), "Published %s@%s\n", resp.Component.Name, rev.Digest)
	}
	for _, f := range rev.Findings {
		output.Writef(p.Err(), "  %s %s: %s\n", strings.ToLower(f.Severity.String()), f.Source, f.Message)
	}
	return resp, nil
}

func kindEnum(kind string) registryv1.ComponentKind {
	switch kind {
	case "terraform":
		return registryv1.ComponentKind_TERRAFORM
	case "helm":
		return registryv1.ComponentKind_HELM
	case "manifests":
		return registryv1.ComponentKind_MANIFESTS
	default:
		return registryv1.ComponentKind_COMPONENT_KIND_UNSPECIFIED
	}
}

func displayCaller(rel string) string {
	if rel == "." {
		return "the root"
	}
	return rel
}

func pinsToProto(pins []bundle.Pin) []*registryv1.Pin {
	out := make([]*registryv1.Pin, 0, len(pins))
	for _, p := range pins {
		out = append(out, &registryv1.Pin{Source: p.Source, Constraint: p.Constraint, Resolved: p.Resolved})
	}
	return out
}
