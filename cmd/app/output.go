package app

import (
	"go.admiral.io/cli/internal/output"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

// appTable is the single column definition shared by list, get and the
// create/update echo. Narrow: NAME DESCRIPTION LABELS AGE; -o wide appends
// ID and CREATED-BY.
var appTable = output.Table[*applicationv1.Application]{
	{Header: "NAME", Cell: func(a *applicationv1.Application) string { return a.Name }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(a *applicationv1.Application) string { return a.Description }},
	{Header: "LABELS", Cell: func(a *applicationv1.Application) string { return output.FormatLabels(a.Labels) }},
	{Header: "AGE", Cell: func(a *applicationv1.Application) string { return output.FormatAge(a.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(a *applicationv1.Application) string { return a.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(a *applicationv1.Application) string { return output.FormatActor(a.CreatedBy) }},
}
