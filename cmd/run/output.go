package run

import (
	"fmt"
	"strings"

	"go.admiral.io/cli/internal/output"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// phasesAsStrings converts a slice of proto phase enums to user-facing
// phase labels ("plan", "apply"). Used for display and for comparison
// against the --phase flag value.
func phasesAsStrings(ps []runv1.RevisionPhase) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p == runv1.RevisionPhase_REVISION_PHASE_UNSPECIFIED {
			continue
		}
		out = append(out, output.FormatEnumKebab(p))
	}
	return out
}

// RunID prefers the human-readable display_id (run-<suffix>) when the server
// has populated it; falls back to a truncated UUID for older records.
func RunID(r *runv1.Run) string {
	if r.DisplayId != "" {
		return r.DisplayId
	}
	if len(r.Id) >= 8 {
		return r.Id[:8]
	}
	return r.Id
}

// orID falls back to the UUID prefix when the server hasn't populated the
// denormalized name (e.g. older row, missing parent).
func orID(name, id string) string {
	if name != "" {
		return name
	}
	if len(id) >= 8 {
		return id[:8]
	}
	if id != "" {
		return id
	}
	return output.None
}

// RunTable is the single column definition shared by list, get and every
// verb that returns a run (plan, apply, cancel, rollback). Runs are addressed
// by ID, so ID is the first column.
var RunTable = output.Table[*runv1.Run]{
	{Header: "ID", Cell: RunID},
	{Header: "STATUS", Cell: func(r *runv1.Run) string { return output.FormatEnum(r.Status) }},
	{Header: "CHANGE SET", Cell: func(r *runv1.Run) string { return orID(r.ChangeSetDisplayId, r.ChangeSetId) }},
	{Header: "TITLE", Truncate: 40, Cell: func(r *runv1.Run) string { return r.ChangeSetTitle }},
	{Header: "AGE", Cell: func(r *runv1.Run) string { return output.FormatAge(r.CreatedAt) }},
	{Header: "ENV", Wide: true, Cell: func(r *runv1.Run) string { return orID(r.EnvironmentName, r.EnvironmentId) }},
	{Header: "MESSAGE", Wide: true, Truncate: 40, Cell: func(r *runv1.Run) string { return r.Message }},
	{Header: "TRIGGERED BY", Wide: true, Cell: func(r *runv1.Run) string { return output.FormatActor(r.TriggeredBy) }},
	{Header: "STARTED", Wide: true, Cell: func(r *runv1.Run) string { return output.FormatTimestamp(r.CreatedAt) }},
	{Header: "COMPLETED", Wide: true, Cell: func(r *runv1.Run) string { return output.FormatTimestamp(r.CompletedAt) }},
}

// revisionTable renders the per-component revisions of a run.
var revisionTable = output.Table[*runv1.Revision]{
	{Header: "COMPONENT", Cell: func(r *runv1.Revision) string { return r.ComponentName }},
	{Header: "KIND", Cell: func(r *runv1.Revision) string { return output.FormatEnumKebab(r.Kind) }},
	{Header: "STATUS", Cell: func(r *runv1.Revision) string { return output.FormatEnum(r.Status) }},
	{Header: "CHANGES", Cell: func(r *runv1.Revision) string { return formatChangeSummary(r.PlanSummary) }},
	{Header: "ERROR", Truncate: 60, Cell: func(r *runv1.Revision) string { return r.ErrorMessage }},
}

// formatChangeSummary renders a plan/apply summary in terraform's symbol
// form, omitting zero counts: "+1 ~2", "-1", or <none> when nothing changed.
func formatChangeSummary(s *runv1.ChangeSummary) string {
	if s == nil {
		return ""
	}
	var parts []string
	if s.Creates > 0 {
		parts = append(parts, fmt.Sprintf("+%d", s.Creates))
	}
	if s.Updates > 0 {
		parts = append(parts, fmt.Sprintf("~%d", s.Updates))
	}
	if s.Deletes > 0 {
		parts = append(parts, fmt.Sprintf("-%d", s.Deletes))
	}
	return strings.Join(parts, " ")
}
