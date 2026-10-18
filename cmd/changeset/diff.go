package changeset

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
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
			for _, l := range leaves(pd.DisplayPath, pd.BeforePresent, pd.BeforeJson, pd.AfterPresent, pd.AfterJson) {
				output.Writef(w, "    %s: %s -> %s\n", l.path, l.before, l.after)
			}
		}
		for _, v := range c.Violations {
			output.Writef(w, "    ! %s: %s\n", v.Path, v.Message)
		}
	}
}

// leaf is one changed value at a full path, each side as side prints it.
type leaf struct{ path, before, after string }

// leaves expands a change to an object into its changed leaves, so a value
// set inside a new object reads at its own path rather than as the object.
// Anything that is not an object on either side is one leaf as given.
func leaves(path string, beforePresent bool, before string, afterPresent bool, after string) []leaf {
	bo, bIsObj := object(beforePresent, before)
	ao, aIsObj := object(afterPresent, after)
	if !bIsObj && !aIsObj || (beforePresent && !bIsObj) || (afterPresent && !aIsObj) {
		return []leaf{{path, side(beforePresent, before), side(afterPresent, after)}}
	}
	keys := make([]string, 0, len(bo)+len(ao))
	for k := range bo {
		keys = append(keys, k)
	}
	for k := range ao {
		if _, ok := bo[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []leaf
	for _, k := range keys {
		bv, bok := bo[k]
		av, aok := ao[k]
		if bok && aok && string(bv) == string(av) {
			continue
		}
		out = append(out, leaves(childPath(path, k), bok, string(bv), aok, string(av))...)
	}
	if len(out) == 0 {
		// An empty object set or cleared has no leaves to show it by.
		return []leaf{{path, side(beforePresent, before), side(afterPresent, after)}}
	}
	return out
}

// object decodes js when it is present and a JSON object.
func object(present bool, js string) (map[string]json.RawMessage, bool) {
	if !present {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(js), &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

// childPath appends a key the way a path is written on the command line:
// a key that is not a plain word is quoted.
func childPath(path, key string) string {
	if !plainKey.MatchString(key) {
		key = strconv.Quote(key)
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

var plainKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// side shows absent apart from null: absent means the default applies,
// null clears it.
func side(present bool, js string) string {
	if !present {
		return "<absent>"
	}
	return js
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
