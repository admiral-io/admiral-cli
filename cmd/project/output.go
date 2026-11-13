package project

import (
	"go.admiral.io/cli/internal/output"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

// projectTable is the single column definition shared by list, get and the
// create/update echo. Narrow: NAME DESCRIPTION LABELS AGE; -o wide appends
// ID and CREATED-BY.
var projectTable = output.Table[*projectv1.Project]{
	{Header: "NAME", Cell: func(a *projectv1.Project) string { return a.Name }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(a *projectv1.Project) string { return a.Description }},
	{Header: "LABELS", Cell: func(a *projectv1.Project) string { return output.FormatLabels(a.Labels) }},
	{Header: "AGE", Cell: func(a *projectv1.Project) string { return output.FormatAge(a.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(a *projectv1.Project) string { return a.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(a *projectv1.Project) string { return output.FormatActor(a.CreatedBy) }},
}
