package credential

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

// credentialTable is the row shared by list, get and the create/update
// echo. The secret has no column: it is never returned.
var credentialTable = output.Table[*credentialv1.Credential]{
	{Header: "NAME", Cell: func(c *credentialv1.Credential) string { return c.Name }},
	{Header: "TYPE", Cell: func(c *credentialv1.Credential) string { return output.FormatEnumKebab(c.Type) }},
	{Header: "HOSTS", Cell: func(c *credentialv1.Credential) string { return formatHosts(c.AllowedHosts) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(c *credentialv1.Credential) string { return c.Description }},
	{Header: "LABELS", Cell: func(c *credentialv1.Credential) string { return output.FormatLabels(c.Labels) }},
	{Header: "AGE", Cell: func(c *credentialv1.Credential) string { return output.FormatAge(c.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(c *credentialv1.Credential) string { return c.Id }},
	{Header: "CREATED-BY", Wide: true, Cell: func(c *credentialv1.Credential) string { return output.FormatActor(c.CreatedBy) }},
}

// formatHosts is the allowed-host guard as a cell: "any" when unset, since
// an empty cell would read as a missing value rather than the wider grant.
func formatHosts(hosts []string) string {
	if len(hosts) == 0 {
		return "any"
	}
	return strings.Join(hosts, ",")
}
