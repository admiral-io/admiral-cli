package component

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newSetOriginCmd(opts *client.Options) *cobra.Command {
	var (
		source    string
		artifact  string
		published bool
		yes       bool
	)

	cmd := &cobra.Command{
		Use:   "set-origin <name> (--source SOURCE [--artifact ARTIFACT] | --published)",
		Short: "Move where a component's new versions come from",
		Long: `Move where a component's new versions come from: a source and an artifact
in it, or published by hand.

Every new version must come from the component's origin, so a component
imported through a source refuses a publish, and one published by hand
refuses an import. Moving the origin changes only where new versions come
from. Existing versions keep their own provenance, and environments
running them are not touched. Moving it is for an organization admin.

--artifact defaults to <name>, except in a git repository, whose root is
empty.`,
		Example: `  # A chart published by hand from now on comes through a source
  admiral component set-origin cert-manager --source jetstack

  # Back to publishing by hand
  admiral component set-origin cert-manager --published`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := checkOriginFlags(source, artifact, published); err != nil {
				return err
			}
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			componentID, err := resolve.Component(cmd.Context(), c.Registry(), name)
			if err != nil {
				return err
			}

			origin, names, err := originTo(cmd, c.Source(), name, source, artifact)
			if err != nil {
				return err
			}
			to := "published by hand"
			if source != "" {
				to = originLabel(origin, names)
			}

			if err := input.Confirm(cmd, yes, fmt.Sprintf("Move %s's origin to %s", name, to)); err != nil {
				return err
			}
			resp, err := c.Registry().SetComponentOrigin(cmd.Context(), &registryv1.SetComponentOriginRequest{
				ComponentId: componentID, Origin: origin,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Writef(p.Err(), "component %q now comes from %s\n", resp.Component.Name, to)
			return p.PrintOne(resp.Component, resp.Component.Name, componentTableWith(names).Render(p, resp.Component))
		},
	}

	cmd.Flags().StringVar(&source, "source", "", "the source new versions come through, by name or ID")
	complete.Flag(cmd, "source", complete.Sources(opts))
	cmd.Flags().StringVar(&artifact, "artifact", "", "what the source lists, when it is not the component's name; for a git source, a directory")
	cmd.Flags().BoolVar(&published, "published", false, "new versions are published by hand")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")

	return cmd
}

func checkOriginFlags(source, artifact string, published bool) error {
	switch {
	case published && source != "":
		return cmderr.Usage("give --source or --published, not both")
	case !published && source == "":
		return cmderr.Usage("give --source, or --published for a component published by hand")
	case published && artifact != "":
		return cmderr.Usage("--artifact goes with --source")
	}
	return nil
}

// originTo is the origin set-origin asks for, with the name of its source.
// No source means published by hand.
func originTo(cmd *cobra.Command, c sourcev1.SourceAPIClient, name, source, artifact string) (*registryv1.ComponentOrigin, map[string]string, error) {
	names := map[string]string{}
	if source == "" {
		return &registryv1.ComponentOrigin{Kind: &registryv1.ComponentOrigin_Published{Published: &registryv1.PublishedOrigin{}}}, names, nil
	}
	src, err := resolve.Source(cmd.Context(), c, source)
	if err != nil {
		return nil, nil, err
	}
	if artifact, err = artifactIn(src, name, artifact); err != nil {
		return nil, nil, err
	}
	names[src.Id] = src.Name
	return &registryv1.ComponentOrigin{Kind: &registryv1.ComponentOrigin_Source{Source: &registryv1.SourceOrigin{
		SourceId: src.Id, Artifact: artifact,
	}}}, names, nil
}

// originLabel is an origin as a table cell: the source by name where it is
// known, then the artifact, or "published" for a component published by
// hand. A component that has no origin yet prints empty.
func originLabel(o *registryv1.ComponentOrigin, names map[string]string) string {
	switch k := o.GetKind().(type) {
	case *registryv1.ComponentOrigin_Published:
		return "published"
	case *registryv1.ComponentOrigin_Source:
		src := k.Source.SourceId
		if n, ok := names[src]; ok {
			src = n
		}
		if k.Source.Artifact == "" {
			return src
		}
		return src + "/" + k.Source.Artifact
	}
	return ""
}

// sourceNames names the sources the components' origins point at, so a
// table shows argo-helm rather than an ID. A failure leaves the IDs.
func sourceNames(cmd *cobra.Command, c sourcev1.SourceAPIClient, comps ...*registryv1.Component) map[string]string {
	names := map[string]string{}
	if !slices.ContainsFunc(comps, func(comp *registryv1.Component) bool { return comp.GetOrigin().GetSource() != nil }) {
		return names
	}
	token := ""
	for {
		resp, err := c.ListSources(cmd.Context(), &sourcev1.ListSourcesRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return names
		}
		for _, s := range resp.Sources {
			names[s.Id] = s.Name
		}
		if resp.NextPageToken == "" {
			return names
		}
		token = resp.NextPageToken
	}
}
