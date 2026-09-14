package changeset

import (
	"strconv"

	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// changeSetID prefers the human-readable display_id (cs-<suffix>) when the
// server has populated it; falls back to a truncated UUID for older records.
func changeSetID(cs *changesetv1.ChangeSet) string {
	if cs.DisplayId != "" {
		return cs.DisplayId
	}
	if len(cs.Id) >= 8 {
		return cs.Id[:8]
	}
	return cs.Id
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
	return id
}

// changeSetTable is the single column definition shared by list, get and
// every verb that returns a change set.
var changeSetTable = output.Table[*changesetv1.ChangeSet]{
	{Header: "ID", Cell: changeSetID},
	{Header: "TITLE", Truncate: 40, Cell: func(cs *changesetv1.ChangeSet) string { return cs.Title }},
	{Header: "APP", Cell: func(cs *changesetv1.ChangeSet) string { return orID(cs.ApplicationName, cs.ApplicationId) }},
	{Header: "ENV", Cell: func(cs *changesetv1.ChangeSet) string { return orID(cs.EnvironmentName, cs.EnvironmentId) }},
	{Header: "STATUS", Cell: func(cs *changesetv1.ChangeSet) string { return output.FormatEnum(cs.Status) }},
	{Header: "ENTRIES", Cell: func(cs *changesetv1.ChangeSet) string { return strconv.Itoa(len(cs.Entries) + len(cs.VariableEntries)) }},
	{Header: "AGE", Cell: func(cs *changesetv1.ChangeSet) string { return output.FormatAge(cs.CreatedAt) }},
	{Header: "CREATED BY", Wide: true, Cell: func(cs *changesetv1.ChangeSet) string { return output.FormatActor(cs.CreatedBy) }},
	{Header: "UPDATED", Wide: true, Cell: func(cs *changesetv1.ChangeSet) string { return output.FormatTimestamp(cs.UpdatedAt) }},
}

// entryTable renders the component entries of a change set.
var entryTable = output.Table[*changesetv1.ChangeSetEntry]{
	{Header: "NAME", Cell: func(e *changesetv1.ChangeSetEntry) string { return e.ComponentName }},
	{Header: "CHANGE TYPE", Cell: func(e *changesetv1.ChangeSetEntry) string { return output.FormatEnum(e.ChangeType) }},
	{Header: "COMPONENT ID", Cell: func(e *changesetv1.ChangeSetEntry) string { return e.ComponentId }},
	{Header: "MODULE", Cell: func(e *changesetv1.ChangeSetEntry) string {
		if e.CatalogItemId == nil {
			return ""
		}
		return orID(e.GetCatalogItemName(), *e.CatalogItemId)
	}},
	{Header: "REF", Cell: func(e *changesetv1.ChangeSetEntry) string { return e.GetRef() }},
}

// varEntryTable renders the variable entries of a change set.
var varEntryTable = output.Table[*changesetv1.ChangeSetVariableEntry]{
	{Header: "KEY", Cell: func(v *changesetv1.ChangeSetVariableEntry) string { return v.Key }},
	{Header: "ACTION", Cell: func(v *changesetv1.ChangeSetVariableEntry) string {
		if v.Value == nil {
			return "DELETE"
		}
		return "SET"
	}},
	{Header: "TYPE", Cell: func(v *changesetv1.ChangeSetVariableEntry) string { return output.TrimEnumPrefix(v.Type) }},
	{Header: "SENSITIVE", Cell: func(v *changesetv1.ChangeSetVariableEntry) string { return strconv.FormatBool(v.Sensitive) }},
	{Header: "VALUE", Truncate: 40, Cell: func(v *changesetv1.ChangeSetVariableEntry) string {
		if v.Value == nil {
			return ""
		}
		if v.Sensitive {
			return "***"
		}
		return *v.Value
	}},
}
