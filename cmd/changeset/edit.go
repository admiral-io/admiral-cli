package changeset

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// maxEdits is the API's cap on edits in one request.
const maxEdits = 100

var (
	changeSetIDPattern = regexp.MustCompile(`^cs-[0-9a-z]{12}$`)
	componentPattern   = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// changeSetID refuses a malformed ID before any sign-in or RPC.
func changeSetID(s string) (string, error) {
	if !changeSetIDPattern.MatchString(s) {
		return "", cmderr.UsageHint("A change set ID is cs- and twelve characters; find it with 'admiral changeset list --env <app>/<env>'.",
			"invalid change set %q", s)
	}
	return s, nil
}

func componentName(s string) error {
	if !componentPattern.MatchString(s) {
		return cmderr.Usage("invalid component name %q: lowercase letters, digits and hyphens, starting with a letter", s)
	}
	return nil
}

// revisionFlag registers --if-revision; ifRevision reads it back as nil
// when it was not passed, since revision 0 is a real precondition.
func revisionFlag(cmd *cobra.Command, dest *int32) {
	cmd.Flags().Int32Var(dest, "if-revision", 0, "refuse unless the change set's head is this revision")
}

func ifRevision(cmd *cobra.Command, n int32) *int32 {
	if !cmd.Flags().Changed("if-revision") {
		return nil
	}
	return &n
}

// edits sends one EditChangeSet, so each command is one revision, and
// prints what it cut. With --plan the answer is the revision's prepare
// instead, and the revision is only confirmed on stderr.
func edits(cmd *cobra.Command, opts *client.Options, csID string, ifRev *int32, po planOptions, es ...*changesetv1.Edit) error {
	if len(es) > maxEdits {
		return cmderr.Usage("%d edits in one command; the limit is %d", len(es), maxEdits)
	}
	if err := po.check(cmd); err != nil {
		return err
	}
	c, err := client.CreateClient(cmd.Context(), opts)
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck

	resp, err := c.ChangeSet().EditChangeSet(cmd.Context(), &changesetv1.EditChangeSetRequest{
		ChangeSetId: csID,
		IfRevision:  ifRev,
		Edits:       es,
		Plan:        po.plan,
	})
	if err != nil {
		return err
	}

	p := output.NewPrinter(cmd, opts.OutputFormat)
	printWarnings(p.Err(), resp.Warnings, resp.Revision.GetViolations())
	output.Confirmed(p.Err(), "change set", csID, "revision "+strconv.Itoa(int(resp.Revision.GetNumber())))
	if po.plan {
		return finishPrepare(cmd, opts, c.ChangeSet(), csID, resp.Prepare, po)
	}
	return p.PrintOne(resp.Revision, revisionName(csID, resp.Revision), revisionTable.Render(p, resp.Revision))
}

// printWarnings goes to stderr: a violation is accepted and stored, so it
// is news, not a failure.
func printWarnings(w io.Writer, warnings []string, violations []*changesetv1.Violation) {
	for _, s := range warnings {
		output.Writef(w, "Warning: %s\n", s)
	}
	for _, v := range violations {
		output.Writef(w, "Violation: %s %s: %s\n", v.Component, v.Path, v.Message)
	}
}

// setEdit turns one --set into an edit. The value reads as a YAML scalar,
// as helm --set does; forceString is --set-string. A null is SetNull, which
// clears a chart default, not SetValue.
func setEdit(a valuesfile.Assignment, forceString bool) (*changesetv1.Edit, error) {
	if err := componentName(a.Component); err != nil {
		return nil, err
	}
	var v any = a.Raw
	if !forceString {
		parsed, err := valuesfile.ParseScalar(a.Raw)
		if err != nil {
			return nil, cmderr.Usage("%s.%s: %v", a.Component, displayPath(a.Path), err)
		}
		v = parsed
	}
	if v == nil {
		return &changesetv1.Edit{Edit: &changesetv1.Edit_SetNull{SetNull: &changesetv1.SetNull{
			Component: a.Component,
			Path:      pathSegments(a.Path),
		}}}, nil
	}
	js, err := valuesfile.EncodeJSON(v)
	if err != nil {
		return nil, cmderr.Usage("%s.%s: %v", a.Component, displayPath(a.Path), err)
	}
	return &changesetv1.Edit{Edit: &changesetv1.Edit_SetValue{SetValue: &changesetv1.SetValue{
		Component: a.Component,
		Path:      pathSegments(a.Path),
		ValueJson: js,
	}}}, nil
}

// setEdits parses --set then --set-string, in that order, as helm applies
// them.
func setEdits(sets, setStrings []string) ([]*changesetv1.Edit, error) {
	out := make([]*changesetv1.Edit, 0, len(sets)+len(setStrings))
	for i, group := range [][]string{sets, setStrings} {
		for _, s := range group {
			a, err := valuesfile.ParseAssignment(s)
			if err != nil {
				return nil, cmderr.Usage("%v", err)
			}
			e, err := setEdit(a, i == 1)
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		}
	}
	return out, nil
}

func pathSegments(keys []string) []*changesetv1.PathSegment {
	out := make([]*changesetv1.PathSegment, len(keys))
	for i, k := range keys {
		out[i] = &changesetv1.PathSegment{Segment: &changesetv1.PathSegment_Key{Key: k}}
	}
	return out
}

// displayPath is for error messages only; the server renders the display
// form it stores.
func displayPath(keys []string) string { return strings.Join(keys, ".") }

func revisionName(csID string, r *changesetv1.Revision) string {
	return fmt.Sprintf("%s/%d", csID, r.GetNumber())
}

// setFlags registers the helm-style pair on create.
func setFlags(cmd *cobra.Command, sets, setStrings *[]string) {
	cmd.Flags().StringArrayVar(sets, "set", nil, "set <component>.<path>=<value>; the value reads as YAML (true, 3, null, !ref db.host) (repeatable)")
	cmd.Flags().StringArrayVar(setStrings, "set-string", nil, "set <component>.<path>=<value> as a string, whatever it looks like (repeatable)")
}
