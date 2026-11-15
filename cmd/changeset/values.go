package changeset

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newValuesCmd(opts *client.Options) *cobra.Command {
	var (
		valuesPath string
		effective  bool
		po         planOptions
	)

	cmd := &cobra.Command{
		Use:               "values <changeset> <component>",
		ValidArgsFunction: complete.Positions(complete.Envs(opts), complete.ComponentsOf(opts, false)),
		Short:             "Download a component's values, or upload them with --values",
		Long: `Download a component's values, or upload them with --values.

A download writes YAML to stdout under a header naming the changeset, the
component and the revision it was taken at. An upload replaces the
component's values with the file's and is refused if the changeset has
moved past that revision, so an edit made meanwhile is never overwritten.
A key missing from the file is removed, so its default applies.

With --effective it prints the whole tree the component will receive
instead: its own defaults, the organization's baseline, what the
environment runs with, then this changeset's edits. Each value is labeled
with the layer it came from. That file is for reading and cannot be
uploaded.`,
		Example: `  admiral changeset values cs-7f2a1c9d0e3b api > api.yaml
  $EDITOR api.yaml
  admiral changeset values cs-7f2a1c9d0e3b api --values api.yaml
  admiral changeset values cs-7f2a1c9d0e3b api --effective`,
		Args: flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			comp := args[1]
			if err := componentName(comp); err != nil {
				return err
			}

			if effective && valuesPath != "" {
				return cmderr.UsageHint("Upload the file you downloaded without --effective.",
					"--effective prints values; it cannot be combined with --values")
			}
			if valuesPath != "" {
				data, err := readValuesFile(cmd, valuesPath)
				if err != nil {
					return err
				}
				upload, rev, fromCS, err := uploadEdit(cmd.ErrOrStderr(), valuesPath, ref, comp, data)
				if err != nil {
					return err
				}
				if err := po.check(cmd); err != nil {
					return err
				}
				c, csID, err := dial(cmd, opts, ref)
				if err != nil {
					return err
				}
				defer c.Close() //nolint:errcheck
				// A project/environment is only known to be the file's changeset once
				// it resolves; an ID was compared before signing in.
				if csID != fromCS {
					return cmderr.Usage("%s was downloaded from changeset %s, not %s (%s)", valuesPath, fromCS, csID, ref)
				}
				return send(cmd, opts, c, csID, &rev, po, upload)
			}

			if po.plan || cmd.Flags().Changed("no-wait") || cmd.Flags().Changed("wait-timeout") {
				return cmderr.UsageHint("Plan an upload: 'admiral changeset values "+ref.String()+" "+comp+" --values "+comp+".yaml --plan'.",
					"--plan applies to an upload with --values")
			}

			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().GetComponentValues(cmd.Context(), &changesetv1.GetComponentValuesRequest{
				ChangeSetId: csID,
				Component:   comp,
				Effective:   effective,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if p.Format.IsMachine() {
				return p.PrintResource(resp, nil)
			}
			tree, err := valuesfile.DecodeJSON(resp.TreeJson)
			if err != nil {
				return err
			}
			if effective {
				out, err := renderEffective(tree, csID, comp, resp.GetSources())
				if err != nil {
					return err
				}
				_, err = p.Out().Write(out)
				return err
			}
			out, err := valuesfile.Render(tree, valuesfile.Header{
				ChangeSet:  csID,
				Component:  comp,
				Revision:   resp.HeadRevision,
				BaseDigest: resp.BaseDigest,
			})
			if err != nil {
				return err
			}
			_, err = p.Out().Write(out)
			return err
		},
	}

	cmd.Flags().StringVar(&valuesPath, "values", "", "upload this file, downloaded earlier, as the component's values")
	cmd.Flags().BoolVar(&effective, "effective", false, "print every value the component will receive, each labeled with the layer it came from")
	planFlags(cmd, &po)

	return cmd
}

// uploadEdit builds the one ReplaceValues an upload sends, the revision
// the file was downloaded at, which the edit sends as its if_revision, and
// the changeset its header names. A file whose header names another
// component, or a changeset other than the ID given, is refused here;
// a stale revision can only be judged by the server, which refuses it.
func uploadEdit(stderr io.Writer, path string, ref changeSetRef, comp string, data []byte) (*changesetv1.Edit, int32, string, error) {
	h, ok := valuesfile.ParseHeader(data)
	fromCS, fromComp := headerNames(data)
	if !ok || fromCS == "" || fromComp == "" {
		return nil, 0, "", cmderr.UsageHint(
			"Download the values first ('admiral changeset values "+ref.String()+" "+comp+" > "+comp+".yaml'), edit that file, then upload it.",
			"%s has no changeset, component and revision header", path)
	}
	if ref.id != "" && fromCS != ref.id {
		return nil, 0, "", cmderr.Usage("%s was downloaded from changeset %s, not %s", path, fromCS, ref.id)
	}
	if fromComp != comp {
		return nil, 0, "", cmderr.Usage("%s holds the values of %s, not %s", path, fromComp, comp)
	}

	tree, warnings, err := valuesfile.Parse(data)
	if err != nil {
		return nil, 0, "", cmderr.Usage("%s: %v", path, err)
	}
	for _, w := range warnings {
		output.Writef(stderr, "Warning: %s: %s\n", path, w)
	}
	js, err := valuesfile.EncodeJSON(tree)
	if err != nil {
		return nil, 0, "", err
	}
	return &changesetv1.Edit{Edit: &changesetv1.Edit_ReplaceValues{ReplaceValues: &changesetv1.ReplaceValues{
		Component: comp,
		TreeJson:  js,
	}}}, h.Revision, fromCS, nil
}

// headerNames reads the changeset and component lines valuesfile.Render
// writes; ParseHeader reads only the revision.
func headerNames(data []byte) (changeSet, component string) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			break
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if v, ok := strings.CutPrefix(line, "changeset:"); ok {
			changeSet = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "component:"); ok {
			component = strings.TrimSpace(v)
		}
	}
	return changeSet, component
}

// layerLabels name each layer as the long help does.
var layerLabels = map[changesetv1.ValueSource_Origin]string{
	changesetv1.ValueSource_ORIGIN_COMPONENT_DEFAULT: "component default",
	changesetv1.ValueSource_ORIGIN_BASELINE:          "baseline",
	changesetv1.ValueSource_ORIGIN_ENVIRONMENT:       "environment",
	changesetv1.ValueSource_ORIGIN_CHANGE_SET:        "changeset",
}

// renderEffective writes the effective tree with each value labeled by its
// layer. Its header has no revision line, so an upload refuses the file.
func renderEffective(tree map[string]any, csID, comp string, sources []*changesetv1.ValueSource) ([]byte, error) {
	labels := map[string]string{}
	for _, src := range sources {
		keys := make([]string, 0, len(src.GetPath()))
		for _, seg := range src.GetPath() {
			keys = append(keys, seg.GetKey())
		}
		l, ok := layerLabels[src.GetOrigin()]
		if !ok {
			l = "unknown layer"
		}
		labels[strings.Join(keys, valuesfile.PathSep)] = l
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# changeset: %s\n# component: %s\n", csID, comp)
	b.WriteString("# The values this component will receive, each labeled with its layer:\n")
	b.WriteString("# component default, then baseline, environment and changeset over it.\n")
	if l, ok := labels[""]; ok {
		fmt.Fprintf(&b, "# Every value comes from: %s\n", l)
	}
	b.WriteString("# For reading only. Upload a file downloaded without --effective.\n")
	body, err := valuesfile.EncodeLabeled(tree, labels)
	if err != nil {
		return nil, err
	}
	b.Write(body)
	return []byte(b.String()), nil
}
