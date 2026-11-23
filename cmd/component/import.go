package component

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/manifest"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newImportCmd(opts *client.Options) *cobra.Command {
	var (
		source      string
		artifact    string
		version     string
		tags        []string
		description string
		labelStrs   []string
		baseline    string
	)

	cmd := &cobra.Command{
		Use:   "import <name> --source SOURCE --version VERSION",
		Short: "Import a copy of an artifact you do not own, through a source",
		Long: `Import a copy of an artifact you do not own, through one of the organization's
sources.

A source is where an upstream is named once: a Helm repository, an OCI
registry, a Terraform registry namespace or a git repository, created with
'admiral source create'. An import fetches one artifact through it, under
the source's own credential and within what the source allows, closes it
the way publish does, and publishes the result as a version with where it
came from recorded as its provenance. What is published is a copy: the
bytes reviewed, whatever upstream does to the tag later. Importing the same
thing twice yields the same version.

<name> is the component. --artifact is what the source lists, and defaults
to <name>: a chart in a Helm repository, a module under a registry
namespace, a name under a container registry. For a git repository it is a
directory, and empty is the root. --version is exact, as the source lists
it. In a terminal a missing --version is asked from the list, with nothing
pre-selected; without one the list is in the refusal.

A chart's version and a module's resolved version become tags whether or
not named.

--baseline-values sets the organization's defaults for the artifact, a
YAML or JSON object laid over its own defaults (a chart's values.yaml, a
module's variable defaults) before any environment's values. They are part
of the version: importing with different baseline values is a new version.`,
		Example: `  # cert-manager from a Helm repository source
  admiral component import cert-manager --source jetstack --version v1.16.2

  # Argo Rollouts; in a terminal, --version is asked from what the source lists
  admiral component import argo-rollouts --source argo-helm

  # One directory of a repository source, under a name of your own
  admiral component import vpc --source infra --artifact modules/vpc --version v2.1.0

  # With the organization's defaults laid over the chart's own
  admiral component import cert-manager --source jetstack --version v1.16.2 --baseline-values cert-manager.yaml`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !manifest.ValidName(name) {
				return cmderr.Usage("component name %q must be lowercase letters, digits and hyphens", name)
			}
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			var baselineJSON string
			if baseline != "" {
				if baselineJSON, err = readBaseline(baseline); err != nil {
					return err
				}
			}
			if source == "" && version == "" {
				return cmderr.Usage("--version is required; with --source it can be asked from the source's list")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			req := &registryv1.ImportComponentRequest{
				Name:               name,
				Artifact:           artifact,
				Version:            version,
				Tags:               tags,
				Description:        description,
				Labels:             labels,
				BaselineValuesJson: baselineJSON,
			}
			importing := name + " " + version
			if source != "" {
				through, err := throughSource(cmd, c.Source(), source, name, artifact, version)
				if err != nil {
					return err
				}
				req.SourceId, req.Artifact, req.Version = through.sourceID, through.artifact, through.version
				importing = through.label
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Writef(p.Err(), "Importing %s\n", importing)
			resp, err := c.Registry().ImportComponent(cmd.Context(), req)
			if err != nil {
				return err
			}
			ver := resp.Version
			// The source's own digest is in the version's provenance; the
			// digest to pin is the version's, which the next line prints.
			if prov := ver.Provenance; prov != nil {
				output.Writef(p.Err(), "Imported %s%s\n", prov.Uri, atRef(prov.Ref))
			}
			if resp.ResolvedFrom != "" {
				output.Writef(p.Err(), "Resolved %s to %s\n", resp.ResolvedFrom, ver.Provenance.GetRef())
			}
			reportPublished(cmd, p, resp.Component, ver, resp.Unchanged)
			return p.PrintOne(ver, resp.Component.Name+"@"+ver.Digest, versionTable.Render(p, ver))
		},
	}

	cmd.Flags().StringVar(&source, "source", "", "the source to import through, by name or ID")
	complete.Flag(cmd, "source", complete.Sources(opts))
	cmd.Flags().StringVar(&artifact, "artifact", "", "what the source lists, when it is not the component's name; for a git source, a directory")
	cmd.Flags().StringVar(&version, "version", "", "the version, exactly as the source lists it")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "tag to set on the version (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "component description, applied when the component is created")
	flags.Label(cmd, &labelStrs, "label to attach when the component is created (key=value, repeatable)")
	cmd.Flags().StringVar(&baseline, "baseline-values", "", "file of default values (YAML or JSON object) laid over the artifact's own")
	_ = cmd.MarkFlagFilename("baseline-values", "yaml", "yml", "json")

	return cmd
}

// readBaseline reads a baseline values file as the JSON object the API
// takes. YAML is read the way a values file is, so JSON reads too.
func readBaseline(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", cmderr.Usage("cannot read --baseline-values: %v", err)
	}
	tree, _, err := valuesfile.Parse(data)
	if err != nil {
		return "", cmderr.Usage("--baseline-values %s: %v", file, err)
	}
	s, err := valuesfile.EncodeJSON(tree)
	if err != nil {
		return "", fmt.Errorf("--baseline-values %s: %w", file, err)
	}
	return s, nil
}

func atRef(ref string) string {
	if ref == "" {
		return ""
	}
	return " at " + ref
}
