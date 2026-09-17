package component

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/bundle"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
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
set once and never moved.`,
		Example: `  # Publish the current directory, named after it
  admiral component publish

  # Publish a module under a version and as latest
  admiral component publish ./modules/cloud-sql --tag v1.2.0 --tag latest

  # A dev build under a floating tag, with a different name
  admiral component publish . --name cloud-sql-dev --tag dev`,
		Args: flags.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if name == "" {
				name = filepath.Base(abs)
			}
			if !namePattern.MatchString(name) {
				return cmderr.Usage("component name %q must be lowercase letters, digits and hyphens (use --name)", name)
			}
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)

			packed, err := bundle.Pack(abs)
			if err != nil {
				return err
			}
			for _, v := range packed.Vendored {
				output.Writef(p.Err(), "Vendored %s (from %s) into %s\n", v.Source, displayCaller(v.Caller), v.Into)
			}
			if len(packed.Bytes) > maxBundleBytes {
				return fmt.Errorf("bundle is %s, over the %s limit", formatSize(int64(len(packed.Bytes))), formatSize(maxBundleBytes))
			}
			output.Writef(p.Err(), "Packed %s: %d files, %s, %s\n", dir, packed.Files, formatSize(int64(len(packed.Bytes))), strings.ToLower(string(packed.Kind)))

			prov := bundle.Describe(cmd.Context(), abs)
			if prov.Dirty {
				output.Writef(p.Err(), "Working copy has uncommitted changes; the revision will say so\n")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Registry().PublishComponent(cmd.Context(), &registryv1.PublishComponentRequest{
				Name:        name,
				Kind:        kindEnum(kind),
				Tags:        tags,
				Description: description,
				Labels:      labels,
				Provenance: &registryv1.Provenance{
					Uri: prov.URI, Commit: prov.Commit, Path: prov.Path, Dirty: prov.Dirty,
				},
				Bundle: packed.Bytes,
			})
			if err != nil {
				return err
			}

			rev := resp.Revision
			if resp.Unchanged {
				output.Writef(p.Err(), "Already published as %s; nothing written\n", shortDigest(rev.Digest))
			} else {
				output.Writef(p.Err(), "Published %s@%s\n", resp.Component.Name, rev.Digest)
			}
			for _, f := range rev.Findings {
				output.Writef(p.Err(), "  %s %s: %s\n", strings.ToLower(f.Severity.String()), f.Source, f.Message)
			}

			return p.PrintOne(rev, resp.Component.Name+"@"+rev.Digest, revisionTable.Render(p, rev))
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "component name (default: the directory's name)")
	flags.Enum(cmd, &kind, "kind", "", "what the directory is, when detection should not decide", "terraform", "helm", "manifests")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "tag to set on the revision (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "component description, applied when the component is created")
	flags.Label(cmd, &labelStrs, "label to attach when the component is created (key=value, repeatable)")

	return cmd
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
