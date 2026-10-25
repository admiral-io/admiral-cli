package changeset

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newDiffCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "diff <changeset>",
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		Short:             "Show what a changeset changes",
		Long: `Show what a changeset's head revision changes: per component, the action,
the version before and after, each changed value before and after, and the
component's contract violations.`,
		Example: `  admiral changeset diff cs-7f2a1c9d0e3b

  # Machine-readable
  admiral changeset diff cs-7f2a1c9d0e3b -o json`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}

			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().DiffChangeSet(cmd.Context(), &changesetv1.DiffChangeSetRequest{ChangeSetId: csID})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if !p.Format.IsMachine() && len(resp.Components) == 0 {
				output.PrintEmpty(p.Err(), "changes", csID)
				return nil
			}
			return p.PrintResource(resp, func(w *tabwriter.Writer) { writeDiff(w, csID, resp) })
		},
	}

	return cmd
}

var actionMark = map[changesetv1.EntryAction]string{
	changesetv1.EntryAction_ENTRY_ACTION_CREATE:  "+",
	changesetv1.EntryAction_ENTRY_ACTION_UPDATE:  "~",
	changesetv1.EntryAction_ENTRY_ACTION_DESTROY: "-",
	changesetv1.EntryAction_ENTRY_ACTION_ORPHAN:  "-",
}

func writeDiff(w io.Writer, csID string, d *changesetv1.DiffChangeSetResponse) {
	output.Writef(w, "%s at revision %d\n", csID, d.Revision)
	for _, c := range d.Components {
		output.Writef(w, "\n%s %s (%s)%s\n", actionMark[c.Action], c.Component,
			output.FormatEnumKebab(c.Action), versionChange(c.OldVersion, c.NewVersion))
		for _, pd := range c.Paths {
			for _, l := range output.Leaves(pd.DisplayPath, pd.BeforePresent, pd.BeforeJson, pd.AfterPresent, pd.AfterJson) {
				output.Writef(w, "    %s: %s -> %s\n", l.Path, l.Before, l.After)
			}
		}
		for _, v := range c.Violations {
			output.Writef(w, "    ! %s: %s\n", v.Path, v.Message)
		}
	}
}

func versionChange(before, after *changesetv1.VersionRef) string {
	switch {
	case before == nil && after == nil:
		return ""
	case before == nil:
		return "  " + versionName(after)
	case after == nil:
		return "  " + versionName(before)
	case before.Digest == after.Digest:
		return "  " + versionName(after)
	default:
		return fmt.Sprintf("  %s -> %s", versionName(before), versionName(after))
	}
}

// versionName is the reference the changeset named, a tag or a digest,
// with the digest it resolved to when the reference is a tag.
func versionName(v *changesetv1.VersionRef) string {
	short := "sha256:" + shortDigest(v.Digest)
	if v.Reference == "" || strings.HasPrefix(v.Reference, "sha256:") {
		return short
	}
	return v.Reference + " (" + short + ")"
}

// shortDigest is the first twelve hex characters, as cmd/component prints
// a digest.
func shortDigest(d string) string {
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return hex
}
