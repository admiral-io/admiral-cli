package changeset

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newValuesCmd(opts *client.Options) *cobra.Command {
	var valuesPath string

	cmd := &cobra.Command{
		Use:   "values <change-set> <component>",
		Short: "Download a component's values, or upload them with --values",
		Long: `Download a component's values, or upload them with --values.

A download writes YAML to stdout under a header naming the change set, the
component and the revision it was taken at. An upload replaces the
component's values with the file's and is refused if the change set has
moved past that revision, so an edit made meanwhile is never overwritten.
A key missing from the file is removed, so its default applies.`,
		Example: `  admiral changeset values cs-7f2a1c9d0e3b api > api.yaml
  $EDITOR api.yaml
  admiral changeset values cs-7f2a1c9d0e3b api --values api.yaml`,
		Args: flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			comp := args[1]
			if err := componentName(comp); err != nil {
				return err
			}

			if valuesPath != "" {
				data, err := readValuesFile(valuesPath)
				if err != nil {
					return err
				}
				upload, err := uploadEdit(cmd.ErrOrStderr(), valuesPath, csID, comp, data)
				if err != nil {
					return err
				}
				// from_revision is the guard; if_revision would say the same.
				return edits(cmd, opts, csID, nil, upload)
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().GetComponentValues(cmd.Context(), &changesetv1.GetComponentValuesRequest{
				ChangeSetId: csID,
				Component:   comp,
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

	return cmd
}

// uploadEdit builds the one ReplaceValues an upload sends. A file whose
// header names another change set or component is refused here; a stale
// revision can only be judged by the server, which refuses it.
func uploadEdit(stderr io.Writer, path, csID, comp string, data []byte) (*changesetv1.Edit, error) {
	h, ok := valuesfile.ParseHeader(data)
	fromCS, fromComp := headerNames(data)
	if !ok || fromCS == "" || fromComp == "" {
		return nil, cmderr.UsageHint(
			"Download the values first ('admiral changeset values "+csID+" "+comp+" > "+comp+".yaml'), edit that file, then upload it.",
			"%s has no change set, component and revision header", path)
	}
	if fromCS != csID {
		return nil, cmderr.Usage("%s was downloaded from change set %s, not %s", path, fromCS, csID)
	}
	if fromComp != comp {
		return nil, cmderr.Usage("%s holds the values of %s, not %s", path, fromComp, comp)
	}

	tree, warnings, err := valuesfile.Parse(data)
	if err != nil {
		return nil, cmderr.Usage("%s: %v", path, err)
	}
	for _, w := range warnings {
		output.Writef(stderr, "Warning: %s: %s\n", path, w)
	}
	js, err := valuesfile.EncodeJSON(tree)
	if err != nil {
		return nil, err
	}
	return &changesetv1.Edit{Edit: &changesetv1.Edit_ReplaceValues{ReplaceValues: &changesetv1.ReplaceValues{
		Component:    comp,
		TreeJson:     js,
		FromRevision: h.Revision,
	}}}, nil
}

// headerNames reads the change set and component lines valuesfile.Render
// writes; ParseHeader reads only the revision.
func headerNames(data []byte) (changeSet, component string) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			break
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if v, ok := strings.CutPrefix(line, "change set:"); ok {
			changeSet = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "component:"); ok {
			component = strings.TrimSpace(v)
		}
	}
	return changeSet, component
}
