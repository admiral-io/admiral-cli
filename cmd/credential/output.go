package credential

import (
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

// credentialTable is the single column definition shared by list, get and
// the create/update echo.
var credentialTable = output.Table[*credentialv1.Credential]{
	{Header: "NAME", Cell: func(c *credentialv1.Credential) string { return c.Name }},
	{Header: "TYPE", Cell: func(c *credentialv1.Credential) string { return output.FormatEnumKebab(c.Type) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(c *credentialv1.Credential) string { return c.Description }},
	{Header: "LABELS", Cell: func(c *credentialv1.Credential) string { return output.FormatLabels(c.Labels) }},
	{Header: "AGE", Cell: func(c *credentialv1.Credential) string { return output.FormatAge(c.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(c *credentialv1.Credential) string { return c.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(c *credentialv1.Credential) string { return output.FormatActor(c.CreatedBy) }},
}
