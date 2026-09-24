package env

import (
	"go.admiral.io/cli/internal/output"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// envTable is the single column definition shared by list, get and the
// create/update echo.
var envTable = output.Table[*environmentv1.Environment]{
	{Header: "NAME", Cell: func(e *environmentv1.Environment) string { return e.Name }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(e *environmentv1.Environment) string { return e.Description }},
	{Header: "LABELS", Cell: func(e *environmentv1.Environment) string { return output.FormatLabels(e.Labels) }},
	{Header: "AGE", Cell: func(e *environmentv1.Environment) string { return output.FormatAge(e.CreatedAt) }},
	{Header: "NAMESPACE", Wide: true, Cell: func(e *environmentv1.Environment) string { return e.GetKubernetes().GetNamespace() }},
	{Header: "CREATE-NAMESPACES", Wide: true, Cell: func(e *environmentv1.Environment) string { return createNamespaces(e.Kubernetes) }},
	{Header: "KUBERNETES", Wide: true, Cell: func(e *environmentv1.Environment) string {
		return capabilitiesSummary(e.GetKubernetes().GetCapabilities())
	}},
	{Header: "ID", Wide: true, Cell: func(e *environmentv1.Environment) string { return e.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(e *environmentv1.Environment) string { return output.FormatActor(e.CreatedBy) }},
}
