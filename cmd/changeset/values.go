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
	var (
		valuesPath string
		po         planOptions
	)

	cmd := &cobra.Command{
		Use:   "values <changeset> <component>",
		Short: "Download a component's values, or upload them with --values",
		Long: `Download a component's values, or upload them with --values.

A download writes YAML to stdout under a header naming the changeset, the
component and the revision it was taken at. An upload replaces the
component's values with the file's and is refused if the changeset has
moved past that revision, so an edit made meanwhile is never overwritten.
A key missing from the file is removed, so its default applies.`,
		Example: `  admiral changeset values cs-7f2a1c9d0e3b api > api.yaml
  $EDITOR api.yaml
  admiral changeset values cs-7f2a1c9d0e3b api --values api.yaml`,
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
				// An app/env is only known to be the file's changeset once
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
