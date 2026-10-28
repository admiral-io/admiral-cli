package run

import (
	"bytes"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// maxDiffCells caps the line comparison of one resource. A resource past it
// is shown as its whole before removed and its whole after added.
const maxDiffCells = 4_000_000

// WriteFullDiff writes each resource the plan changes whole, as YAML: the
// object as the apply leaves it, with the lines it changes marked - and +.
// That is what lands, the cluster's defaults included, not the chart's
// output. A create is all +, a delete all -. A sensitive value, which the
// agent never sends, is shown as <sensitive>; a value known only once
// applied as <known after apply>. What follows the resources is as
// WriteDiff writes it.
func WriteFullDiff(w io.Writer, d *agentv1.PlanDiff, plan *runv1.PlanPhase) {
	w = &noLeadingBlank{w: w}
	for _, rd := range d.GetResources() {
		a := rd.GetAction()
		output.Writef(w, "\n%s %s: %s (%s)%s\n", actionMark[a], rd.GetComponent(),
			ResourceName(rd.GetResource()), actionWords(a), terraformNote(rd.GetTerraform()))
		before, after := resourceYAML(rd)
		for _, l := range lineDiff(before, after) {
			output.Writef(w, "  %s\n", strings.TrimRight(l, " "))
		}
	}
	writeTail(w, d, plan)
}

// resourceYAML is a resource's before and after as YAML lines, with its
// redacted and unknown values marked.
func resourceYAML(rd *agentv1.ResourceDiff) (before, after []string) {
	var bt, at any
	if rd.GetBefore() != nil {
		bt = rd.GetBefore().AsInterface()
	}
	if rd.GetAfter() != nil {
		at = rd.GetAfter().AsInterface()
	}
	for _, sp := range rd.GetSensitivePaths() {
		bt = setAt(bt, sp.GetPath().GetSegments(), "<sensitive>")
		mark := "<sensitive>"
		if sp.GetChanged() {
			mark = "<sensitive, changed>"
		}
		at = setAt(at, sp.GetPath().GetSegments(), mark)
	}
	for _, p := range rd.GetUnknownPaths() {
		at = setAt(at, p.GetSegments(), "<known after apply>")
	}
	return yamlLines(bt, rd.GetBefore() != nil), yamlLines(at, rd.GetAfter() != nil)
}

// setAt replaces the value at path in v with s, if the path is there.
func setAt(v any, path []*agentv1.DiffPathSegment, s string) any {
	if len(path) == 0 {
		return s
	}
	seg := path[0]
	switch t := v.(type) {
	case map[string]any:
		if _, ok := seg.GetSegment().(*agentv1.DiffPathSegment_Key); ok {
			if child, there := t[seg.GetKey()]; there {
				t[seg.GetKey()] = setAt(child, path[1:], s)
			}
		}
	case []any:
		if _, ok := seg.GetSegment().(*agentv1.DiffPathSegment_Index); ok {
			if i := int(seg.GetIndex()); i >= 0 && i < len(t) {
				t[i] = setAt(t[i], path[1:], s)
			}
		}
	}
	return v
}

func yamlLines(v any, present bool) []string {
	if !present {
		return nil
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return []string{"<unprintable: " + err.Error() + ">"}
	}
	_ = enc.Close()
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

// lineDiff marks each line of a and b as kept ("  "), removed ("- ") or
// added ("+ "), in order, keeping every unchanged line for context.
func lineDiff(a, b []string) []string {
	if len(a)*len(b) > maxDiffCells {
		out := make([]string, 0, len(a)+len(b))
		for _, l := range a {
			out = append(out, "- "+l)
		}
		for _, l := range b {
			out = append(out, "+ "+l)
		}
		return out
	}
	// lcs[i][j] is the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	out := make([]string, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, "  "+a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, "- "+a[i])
	}
	for ; j < len(b); j++ {
		out = append(out, "+ "+b[j])
	}
	return out
}
