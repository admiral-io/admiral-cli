package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

var actionMark = map[agentv1.ResourceChange_Action]string{
	agentv1.ResourceChange_ACTION_CREATE:    "+",
	agentv1.ResourceChange_ACTION_UPDATE:    "~",
	agentv1.ResourceChange_ACTION_REPLACE:   "±",
	agentv1.ResourceChange_ACTION_DESTROY:   "-",
	agentv1.ResourceChange_ACTION_ORPHAN:    "-",
	agentv1.ResourceChange_ACTION_TAKE_OVER: "~",
}

// WriteDiff writes a plan's diffs resource by resource, as 'run diff' and
// the changeset verbs that show a plan before acting print them: each
// changed value at its path, before and after. A create lists what it sets,
// a delete is one line, and a takeover names the fields it takes from
// another manager. Changes made outside Admiral, the outputs the apply
// would change, and why a failed plan failed follow. plan says which
// components the plan removes outright; nil is fine.
func WriteDiff(w io.Writer, d *agentv1.PlanDiff, plan *runv1.PlanPhase) {
	w = &noLeadingBlank{w: w}
	writeResources(w, d, false)
	writeTail(w, d, plan)
}

// WriteBriefDiff writes the diff as WriteDiff does, shortened for reading
// before approving. A takeover counts the fields it takes instead of
// naming each one, a Kubernetes create counts what it sets, since most of
// that is the API server's defaults, and the resources a component changes
// the same way are listed once over the change they share. When it leaves
// anything out it says where the whole diff is: 'admiral run diff runID'.
// What changed outside Admiral is never shortened.
func WriteBriefDiff(w io.Writer, d *agentv1.PlanDiff, plan *runv1.PlanPhase, runID string) {
	w = &noLeadingBlank{w: w}
	if writeResources(w, d, true) && runID != "" {
		output.Writef(w, "\nEvery field: admiral run diff %s\n", runID)
	}
	writeTail(w, d, plan)
}

// writeResources writes the resources and reports whether brief left anything out.
func writeResources(w io.Writer, d *agentv1.PlanDiff, brief bool) bool {
	if !brief {
		for _, rd := range d.GetResources() {
			writeResource(w, rd, "")
		}
		return false
	}
	type group struct {
		first *agentv1.ResourceDiff
		names []string
		body  string
		taken []managerCount
	}
	var groups []*group
	byKey := map[string]*group{}
	shortened := false
	for _, rd := range d.GetResources() {
		body, cut := resourceBody(rd, "    ", true)
		taken := takenCounts(rd)
		shortened = shortened || cut || len(taken) > 0
		key := strings.Join([]string{rd.GetComponent(), rd.GetAction().String(), managers(taken), body}, "\x00")
		g := byKey[key]
		if g == nil {
			g = &group{first: rd, body: body}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.names = append(g.names, ResourceName(rd.GetResource())+terraformNote(rd.GetTerraform()))
		g.taken = addCounts(g.taken, taken)
	}
	for _, g := range groups {
		a := g.first.GetAction()
		if len(g.names) == 1 {
			output.Writef(w, "\n%s %s: %s (%s)\n", actionMark[a], g.first.GetComponent(), g.names[0], actionWords(a))
			output.Writef(w, "%s", g.body)
			for _, t := range g.taken {
				output.Writef(w, "    takes %s from %s\n", fields(t.n), t.manager)
			}
			continue
		}
		output.Writef(w, "\n%s %s: %d resources (%s)\n", actionMark[a], g.first.GetComponent(), len(g.names), actionWords(a))
		for _, n := range g.names {
			output.Writef(w, "    %s\n", n)
		}
		if g.body == "" && len(g.taken) == 0 {
			continue
		}
		output.Writef(w, "  on each:\n%s", g.body)
		for _, t := range g.taken {
			output.Writef(w, "    takes its fields from %s, %d in all\n", t.manager, t.n)
		}
	}
	return shortened
}

// writeTail writes what follows the resources: the namespaces a removal
// leaves, drift, outputs and errors.
func writeTail(w io.Writer, d *agentv1.PlanDiff, plan *runv1.PlanPhase) {
	removed := map[string]bool{}
	for _, c := range plan.GetComponents() {
		all := len(c.GetChanges()) > 0
		for _, rc := range c.GetChanges() {
			all = all && rc.GetAction() == agentv1.ResourceChange_ACTION_DESTROY
		}
		removed[c.GetComponent()] = all
	}
	kept := map[string]bool{}
	var namespaces []string
	for _, rd := range d.GetResources() {
		k := rd.GetResource().GetKubernetes()
		if rd.GetAction() == agentv1.ResourceChange_ACTION_DESTROY && k.GetNamespace() != "" &&
			removed[rd.GetComponent()] && !kept[k.GetNamespace()] {
			kept[k.GetNamespace()] = true
			namespaces = append(namespaces, k.GetNamespace())
		}
	}
	for _, ns := range namespaces {
		output.Writef(w, "\nThe namespace %s stays; Admiral never deletes a namespace.\n", ns)
	}
	if len(d.GetDrift()) > 0 {
		output.Writef(w, "\nChanged outside Admiral since the last apply:\n")
		for _, rd := range d.GetDrift() {
			writeResource(w, rd, "  ")
		}
	}
	if len(d.GetOutputs()) > 0 {
		output.Writef(w, "\nOutputs:\n")
		for _, o := range d.GetOutputs() {
			output.Writef(w, "  %s %s.%s: %s -> %s\n", actionMark[o.GetAction()], o.GetComponent(), o.GetName(),
				outputSide(o.GetBefore(), o.GetSensitive(), false), outputSide(o.GetAfter(), o.GetSensitive(), o.GetUnknown()))
		}
	}
	if len(d.GetErrors()) > 0 {
		output.Writef(w, "\nErrors:\n")
		for _, e := range d.GetErrors() {
			where := e.GetComponent()
			if e.GetResource() != nil {
				where += ": " + ResourceName(e.GetResource())
			}
			output.Writef(w, "  ! %s: %s\n", where, e.GetSummary())
			if e.GetLocation() != "" {
				output.Writef(w, "      at %s\n", e.GetLocation())
			}
			for _, l := range strings.Split(strings.TrimSpace(e.GetDetail()), "\n") {
				if l != "" {
					output.Writef(w, "      %s\n", l)
				}
			}
		}
	}
}

func writeResource(w io.Writer, rd *agentv1.ResourceDiff, indent string) {
	a := rd.GetAction()
	output.Writef(w, "\n%s%s %s: %s (%s)%s\n", indent, actionMark[a], rd.GetComponent(),
		ResourceName(rd.GetResource()), actionWords(a), terraformNote(rd.GetTerraform()))
	body, _ := resourceBody(rd, indent+"    ", false)
	output.Writef(w, "%s", body)
}

// resourceBody writes what a resource's change sets, each line at pad. Brief
// leaves out the fields a takeover takes, for the caller to count, and counts
// a Kubernetes create's fields instead of listing them; cut reports the
// latter.
func resourceBody(rd *agentv1.ResourceDiff, pad string, brief bool) (body string, cut bool) {
	a := rd.GetAction()
	if a == agentv1.ResourceChange_ACTION_DESTROY || a == agentv1.ResourceChange_ACTION_ORPHAN {
		return "", false
	}
	var b strings.Builder
	w := &b
	sensitive := map[string]bool{}
	for _, sp := range rd.GetSensitivePaths() {
		sensitive[pathString(sp.GetPath())] = true
	}
	unknown := pathSet(rd.GetUnknownPaths())
	replace := pathSet(rd.GetReplacePaths())
	bp, bj := valueJSON(rd.GetBefore())
	ap, aj := valueJSON(rd.GetAfter())
	leaves := output.Leaves("", bp, bj, ap, aj)
	if brief && a == agentv1.ResourceChange_ACTION_CREATE && rd.GetResource().GetKubernetes() != nil {
		n := 0
		for _, l := range leaves {
			if !under(l.Path, sensitive) {
				n++
			}
		}
		if n == 0 {
			return "", false
		}
		return pad + "sets " + fields(n) + "\n", true
	}
	for _, l := range leaves {
		if under(l.Path, sensitive) {
			continue
		}
		after := l.After
		if under(l.Path, unknown) {
			after = "<known after apply>"
		}
		note := ""
		if under(l.Path, replace) {
			note = "  (forces replacement)"
		}
		if a == agentv1.ResourceChange_ACTION_CREATE {
			output.Writef(w, "%s%s: %s%s\n", pad, l.Path, after, note)
			continue
		}
		output.Writef(w, "%s%s: %s -> %s%s\n", pad, l.Path, l.Before, after, note)
	}
	for _, sp := range rd.GetSensitivePaths() {
		if sp.GetChanged() {
			output.Writef(w, "%s%s: <sensitive, changed>\n", pad, pathString(sp.GetPath()))
		}
	}
	if !brief {
		for _, t := range rd.GetKubernetes().GetTaken() {
			output.Writef(w, "%stakes %s from %s\n", pad, pathString(t.GetPath()), t.GetManager())
		}
	}
	return b.String(), false
}

type managerCount struct {
	manager string
	n       int
}

// takenCounts counts the fields a takeover takes from each manager, in the
// order the managers first appear.
func takenCounts(rd *agentv1.ResourceDiff) []managerCount {
	var out []managerCount
	for _, t := range rd.GetKubernetes().GetTaken() {
		out = addCounts(out, []managerCount{{t.GetManager(), 1}})
	}
	return out
}

func addCounts(to, from []managerCount) []managerCount {
	for _, f := range from {
		i := slices.IndexFunc(to, func(c managerCount) bool { return c.manager == f.manager })
		if i < 0 {
			to = append(to, f)
			continue
		}
		to[i].n += f.n
	}
	return to
}

func managers(cs []managerCount) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.manager
	}
	return strings.Join(names, ",")
}

func fields(n int) string {
	if n == 1 {
		return "1 field"
	}
	return fmt.Sprintf("%d fields", n)
}

func actionWords(a agentv1.ResourceChange_Action) string {
	switch a {
	case agentv1.ResourceChange_ACTION_ORPHAN:
		return "orphan, left in place"
	case agentv1.ResourceChange_ACTION_TAKE_OVER:
		return "take over"
	}
	return output.FormatEnumKebab(a)
}

func terraformNote(t *agentv1.TerraformChange) string {
	var notes []string
	if t.GetPreviousAddress() != "" {
		notes = append(notes, "moved from "+t.GetPreviousAddress())
	}
	if t.GetImporting() {
		notes = append(notes, "imported")
	}
	if t.GetDeposed() != "" {
		notes = append(notes, "deposed object "+t.GetDeposed())
	}
	if len(notes) == 0 {
		return ""
	}
	return "  " + strings.Join(notes, ", ")
}

// ResourceName names a Kubernetes object by its API version, kind and
// namespaced name, and a Terraform resource by its address.
func ResourceName(r *agentv1.ResourceRef) string {
	if t := r.GetTerraform(); t != nil {
		return t.GetAddress()
	}
	k := r.GetKubernetes()
	name := k.GetName()
	if k.GetNamespace() != "" {
		name = k.GetNamespace() + "/" + name
	}
	return k.GetApiVersion() + " " + k.GetKind() + " " + name
}

func valueJSON(v *structpb.Value) (bool, string) {
	if v == nil {
		return false, ""
	}
	b, err := json.Marshal(v.AsInterface())
	if err != nil {
		return false, ""
	}
	return true, string(b)
}

func outputSide(v *structpb.Value, sensitive, unknown bool) string {
	switch {
	case unknown:
		return "<known after apply>"
	case sensitive:
		return "<sensitive>"
	}
	_, js := valueJSON(v)
	if js == "" {
		return output.Absent
	}
	return js
}

// pathString writes a path the way output.Leaves does, so the two compare.
func pathString(p *agentv1.DiffPath) string {
	s := ""
	for _, seg := range p.GetSegments() {
		if _, ok := seg.GetSegment().(*agentv1.DiffPathSegment_Index); ok {
			s = output.IndexPath(s, int(seg.GetIndex()))
			continue
		}
		s = output.ChildPath(s, seg.GetKey())
	}
	return s
}

func pathSet(ps []*agentv1.DiffPath) map[string]bool {
	set := make(map[string]bool, len(ps))
	for _, p := range ps {
		set[pathString(p)] = true
	}
	return set
}

// under reports whether path is one of set's paths or inside one.
func under(path string, set map[string]bool) bool {
	for p := range set {
		if path == p || strings.HasPrefix(path, p+".") || strings.HasPrefix(path, p+"[") {
			return true
		}
	}
	return false
}

// noLeadingBlank drops the newlines a diff opens with: each group writes the
// blank line before it, and the first has nothing to be set apart from. The
// caller sets the whole diff apart from what precedes it.
type noLeadingBlank struct {
	w       io.Writer
	started bool
}

func (n *noLeadingBlank) Write(p []byte) (int, error) {
	if n.started {
		return n.w.Write(p)
	}
	t := bytes.TrimLeft(p, "\n")
	if len(t) == 0 {
		return len(p), nil
	}
	n.started = true
	if _, err := n.w.Write(t); err != nil {
		return 0, err
	}
	return len(p), nil
}
