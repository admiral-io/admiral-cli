package changeset

import (
	"strconv"

	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// changeSetTable is the narrow row shared by list, get and the echo after
// create. The style guide's ENV and ENTRIES columns wait on the API: a
// ChangeSet carries only the environment's ID and no entry count.
var changeSetTable = output.Table[*changesetv1.ChangeSet]{
	{Header: "ID", Cell: func(c *changesetv1.ChangeSet) string { return c.Id }},
	{Header: "TITLE", Truncate: 40, Cell: func(c *changesetv1.ChangeSet) string { return c.Title }},
	{Header: "STATUS", Cell: func(c *changesetv1.ChangeSet) string { return output.FormatEnumKebab(c.Status) }},
	{Header: "REVISION", Cell: func(c *changesetv1.ChangeSet) string { return strconv.Itoa(int(c.HeadRevision)) }},
	{Header: "AGE", Cell: func(c *changesetv1.ChangeSet) string { return output.FormatAge(c.CreatedAt) }},
	{Header: "ENVIRONMENT-ID", Wide: true, Cell: func(c *changesetv1.ChangeSet) string { return c.EnvironmentId }},
	{Header: "CREATED-BY", Wide: true, Cell: func(c *changesetv1.ChangeSet) string { return output.FormatActor(c.CreatedBy) }},
	{Header: "UPDATED", Wide: true, Cell: func(c *changesetv1.ChangeSet) string { return output.FormatAge(c.UpdatedAt) }},
}

// revisionTable is what every edit answers with.
var revisionTable = output.Table[*changesetv1.Revision]{
	{Header: "REVISION", Cell: func(r *changesetv1.Revision) string { return strconv.Itoa(int(r.Number)) }},
	{Header: "ENTRIES", Cell: func(r *changesetv1.Revision) string { return strconv.Itoa(len(r.Entries)) }},
	{Header: "VIOLATIONS", Cell: func(r *changesetv1.Revision) string { return strconv.Itoa(len(r.Violations)) }},
	{Header: "AGE", Cell: func(r *changesetv1.Revision) string { return output.FormatAge(r.CreatedAt) }},
	{Header: "CAUSE", Wide: true, Cell: func(r *changesetv1.Revision) string { return output.FormatEnumKebab(r.Cause) }},
	{Header: "BASE-GENERATION", Wide: true, Cell: func(r *changesetv1.Revision) string { return strconv.FormatInt(r.BaseGeneration, 10) }},
	{Header: "CREATED-BY", Wide: true, Cell: func(r *changesetv1.Revision) string { return output.FormatActor(r.CreatedBy) }},
}
