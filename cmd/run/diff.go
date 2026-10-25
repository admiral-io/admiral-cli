package run

import (
	"encoding/json"
	"io"
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
		writeResource(w, rd, "")
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
	if a == agentv1.ResourceChange_ACTION_DESTROY || a == agentv1.ResourceChange_ACTION_ORPHAN {
		return
	}
	pad := indent + "    "
	sensitive := map[string]bool{}
	for _, sp := range rd.GetSensitivePaths() {
		sensitive[pathString(sp.GetPath())] = true
	}
	unknown := pathSet(rd.GetUnknownPaths())
	replace := pathSet(rd.GetReplacePaths())
	bp, bj := valueJSON(rd.GetBefore())
	ap, aj := valueJSON(rd.GetAfter())
	for _, l := range output.Leaves("", bp, bj, ap, aj) {
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
	for _, t := range rd.GetKubernetes().GetTaken() {
		output.Writef(w, "%stakes %s from %s\n", pad, pathString(t.GetPath()), t.GetManager())
	}
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
