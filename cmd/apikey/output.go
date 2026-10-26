package apikey

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	apikeyv1 "go.admiral.io/sdk/proto/admiral/api/apikey/v1"
)

// apiKeyTable is the row list shows. The secret has no column: it is never
// returned after create.
var apiKeyTable = output.Table[*apikeyv1.ApiKey]{
	{Header: "NAME", Cell: func(k *apikeyv1.ApiKey) string { return k.Name }},
	{Header: "SCOPES", Truncate: 40, Cell: func(k *apikeyv1.ApiKey) string { return strings.Join(k.Scopes, ",") }},
	{Header: "STATUS", Cell: func(k *apikeyv1.ApiKey) string { return output.FormatEnumKebab(k.Status) }},
	{Header: "LAST-USED", Cell: lastUsed},
	{Header: "EXPIRES", Cell: expires},
	{Header: "AGE", Cell: func(k *apikeyv1.ApiKey) string { return output.FormatAge(k.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(k *apikeyv1.ApiKey) string { return k.Id }},
}

func lastUsed(k *apikeyv1.ApiKey) string {
	if k.GetLastUsedAt() == nil {
		return "never"
	}
	return output.FormatAge(k.GetLastUsedAt()) + " ago"
}

func expires(k *apikeyv1.ApiKey) string {
	if k.GetExpiresAt() == nil {
		return "never"
	}
	return output.FormatTimestamp(k.GetExpiresAt())
}
