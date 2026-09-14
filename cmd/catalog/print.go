package catalog

import (
	"go.admiral.io/cli/internal/output"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

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

// catalogItemTable is the single column definition shared by list, get and
// the create/update echo.
var catalogItemTable = output.Table[*catalogv1.CatalogItem]{
	{Header: "NAME", Cell: func(m *catalogv1.CatalogItem) string { return m.Name }},
	{Header: "TYPE", Cell: func(m *catalogv1.CatalogItem) string { return output.FormatEnumKebab(m.Type) }},
	{Header: "SOURCE", Cell: func(m *catalogv1.CatalogItem) string { return orID(m.SourceName, m.SourceId) }},
	{Header: "REF", Cell: func(m *catalogv1.CatalogItem) string { return refOrDefault(m.Ref) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(m *catalogv1.CatalogItem) string { return m.Description }},
	{Header: "LABELS", Cell: func(m *catalogv1.CatalogItem) string { return output.FormatLabels(m.Labels) }},
	{Header: "AGE", Cell: func(m *catalogv1.CatalogItem) string { return output.FormatAge(m.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(m *catalogv1.CatalogItem) string { return m.Id }},
	{Header: "ROOT", Wide: true, Cell: func(m *catalogv1.CatalogItem) string { return m.Root }},
	{Header: "PATH", Wide: true, Cell: func(m *catalogv1.CatalogItem) string { return m.Path }},
	{Header: "CREATED BY", Wide: true, Cell: func(m *catalogv1.CatalogItem) string { return output.FormatActor(m.CreatedBy) }},
}

func refOrDefault(ref string) string {
	if ref == "" {
		return "(default)"
	}
	return ref
}
