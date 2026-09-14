package changeset

import (
	"fmt"
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
		Use:   "diff <id>",
		Short: "Show what a change set would change",
		Long: `Compute the structural diff for a change set: per-component entry deltas
relative to the env's current HEAD, per-key variable deltas, and the deployed
components whose values_template references a component touched by this change set.

Sensitive values are masked: changed-but-masked entries print "(sensitive)"
without revealing the value.`,
		Example: `  admiral changeset diff cs-3k7m9p2q4rvw
  admiral changeset diff cs-3k7m9p2q4rvw -o json`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().DiffChangeSet(cmd.Context(), &changesetv1.DiffChangeSetRequest{
				ChangeSetId: args[0],
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				printDiff(w, resp.GetDiff())
			})
		},
	}
	return cmd
}

// printDiff renders the structured diff in three sections: ENTRIES,
// VARIABLES, DOWNSTREAM IMPACT. Each section is omitted when empty so a
// no-op diff prints just the "No changes." line.
func printDiff(w *tabwriter.Writer, d *changesetv1.ChangeSetDiff) {
	if d == nil ||
		(len(d.GetEntries()) == 0 &&
			len(d.GetVariables()) == 0 &&
			len(d.GetDownstream()) == 0) {
		output.Writeln(w, "No changes.")
		return
	}

	if len(d.GetEntries()) > 0 {
		output.Writeln(w, "ENTRIES:")
		output.Writeln(w, "NAME\tCHANGE\tMODULE\tREF\tDETAILS")
		for _, e := range d.GetEntries() {
			printEntryDiff(w, e)
		}
	}

	if len(d.GetVariables()) > 0 {
		if len(d.GetEntries()) > 0 {
			output.Writeln(w, "")
		}
		output.Writeln(w, "VARIABLES:")
		output.Writeln(w, "KEY\tCHANGE\tOLD\tNEW")
		for _, v := range d.GetVariables() {
			printVariableDiff(w, v)
		}
	}

	if len(d.GetDownstream()) > 0 {
		if len(d.GetEntries()) > 0 || len(d.GetVariables()) > 0 {
			output.Writeln(w, "")
		}
		output.Writeln(w, "DOWNSTREAM IMPACT:")
		output.Writeln(w, "NAME\tAFFECTED BY")
		for _, di := range d.GetDownstream() {
			output.Writef(w, "%s\t%s\n", di.GetComponentName(), strings.Join(di.GetAffectedBy(), ", "))
		}
	}
}

// printEntryDiff renders a single EntryDiff row plus indented sub-rows for
// values and depends_on changes when present. The header columns describe
// the high-level intent (change type, module, version); details that don't
// fit on one line print as continuation rows beneath it.
func printEntryDiff(w *tabwriter.Writer, e *changesetv1.EntryDiff) {
	mod := output.None
	ver := output.None
	if m := e.GetCatalogItem(); m != nil {
		mod = formatModuleSide(m.CatalogItemNameOld, m.CatalogItemNameNew, m.CatalogItemIdOld, m.CatalogItemIdNew)
		ver = formatRefSide(m.RefOld, m.RefNew)
	}

	details := summarizeEntryDetails(e)
	output.Writef(w, "%s\t%s\t%s\t%s\t%s\n",
		e.GetComponentName(),
		output.FormatEnum(e.GetChangeType()),
		mod,
		ver,
		details,
	)

	for _, vd := range e.GetValues() {
		oldStr, newStr := valueDiffSides(vd)
		output.Writef(w, "  %s\t%s\t%s\t%s\t%s\n",
			vd.GetKey(),
			output.FormatEnum(vd.GetChangeType()),
			output.Truncate(oldStr, 40),
			output.Truncate(newStr, 40),
			"",
		)
	}
}

// summarizeEntryDetails packs the per-entry "things that changed but don't
// have their own column" into one short cell: depends_on edits, description
// edit, count of value-template changes.
func summarizeEntryDetails(e *changesetv1.EntryDiff) string {
	var parts []string
	if n := len(e.GetValues()); n > 0 {
		parts = append(parts, plural(n, "value"))
	}
	if added := e.GetDependsOnAdded(); len(added) > 0 {
		parts = append(parts, "+deps:"+strings.Join(added, ","))
	}
	if removed := e.GetDependsOnRemoved(); len(removed) > 0 {
		parts = append(parts, "-deps:"+strings.Join(removed, ","))
	}
	if e.DescriptionOld != nil || e.DescriptionNew != nil {
		parts = append(parts, "description")
	}
	if len(parts) == 0 {
		return output.None
	}
	return strings.Join(parts, "; ")
}

func formatModuleSide(oldName, newName, oldID, newID *string) string {
	o := preferStr(oldName, oldID)
	n := preferStr(newName, newID)
	if o == "" && n == "" {
		return output.None
	}
	if o == n || o == "" {
		if n == "" {
			return o
		}
		return n
	}
	return o + " -> " + n
}

func formatRefSide(oldV, newV *string) string {
	o := derefOrEmpty(oldV)
	n := derefOrEmpty(newV)
	if o == "" && n == "" {
		return output.None
	}
	if o == n {
		return n
	}
	if o == "" {
		return n
	}
	if n == "" {
		return o + " -> (unset)"
	}
	return o + " -> " + n
}

// valueDiffSides resolves the old/new strings for a ValueDiff, including the
// sensitive-mask path. Truncation happens at the call site so the caller can
// tune column width.
func valueDiffSides(vd *changesetv1.ValueDiff) (string, string) {
	if vd.GetSensitive() {
		return "(sensitive)", "(sensitive)"
	}
	return derefOrEmpty(vd.Old), derefOrEmpty(vd.New)
}

func printVariableDiff(w *tabwriter.Writer, v *changesetv1.VariableDiff) {
	oldStr := derefOrEmpty(v.Old)
	newStr := derefOrEmpty(v.New)
	if v.GetSensitive() {
		oldStr = "(sensitive)"
		newStr = "(sensitive)"
	}
	if oldStr == "" {
		oldStr = output.None
	}
	if newStr == "" {
		newStr = output.None
	}
	output.Writef(w, "%s\t%s\t%s\t%s\n",
		v.GetKey(),
		output.FormatEnum(v.GetChangeType()),
		output.Truncate(oldStr, 40),
		output.Truncate(newStr, 40),
	)
}

func preferStr(primary, fallback *string) string {
	if primary != nil && *primary != "" {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return ""
}

func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
