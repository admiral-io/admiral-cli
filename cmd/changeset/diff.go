package changeset

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newDiffCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <change-set>",
		Short: "Show what a change set changes",
		Long: `Show what a change set's head revision changes: per component, the action,
the pin before and after, each changed value before and after, and the
component's contract violations.`,
		Example: `  admiral changeset diff cs-7f2a1c9d0e3b

  # Machine-readable
  admiral changeset diff cs-7f2a1c9d0e3b -o json`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
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
	changesetv1.EntryAction_CREATE:  "+",
	changesetv1.EntryAction_UPDATE:  "~",
	changesetv1.EntryAction_DESTROY: "-",
}

func writeDiff(w io.Writer, csID string, d *changesetv1.DiffChangeSetResponse) {
	output.Writef(w, "%s at revision %d\n", csID, d.Revision)
	for _, c := range d.Components {
		output.Writef(w, "\n%s %s (%s)%s\n", actionMark[c.Action], c.Component,
			output.FormatEnumKebab(c.Action), pinChange(c.OldPin, c.NewPin))
		for _, pd := range c.Paths {
			output.Writef(w, "    %s: %s -> %s\n", pd.DisplayPath,
				side(pd.BeforePresent, pd.BeforeJson), side(pd.AfterPresent, pd.AfterJson))
		}
		for _, v := range c.Violations {
			output.Writef(w, "    ! %s: %s\n", v.Path, v.Message)
		}
	}
}

// side shows absent apart from null: absent means the default applies,
// null clears it.
func side(present bool, js string) string {
	if !present {
		return "<absent>"
	}
	return js
}

func pinChange(before, after *changesetv1.Pin) string {
	switch {
	case before == nil && after == nil:
		return ""
	case before == nil:
		return "  " + pinName(after)
	case after == nil:
		return "  " + pinName(before)
	case before.Digest == after.Digest:
		return "  " + pinName(after)
	default:
		return fmt.Sprintf("  %s -> %s", pinName(before), shortDigest(after.Digest))
	}
}

func pinName(p *changesetv1.Pin) string {
	return p.Name + "@" + shortDigest(p.Digest)
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
