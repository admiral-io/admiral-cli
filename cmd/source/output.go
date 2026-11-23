package source

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// sourceTable is the row shared by list, get and the create/update echo.
var sourceTable = output.Table[*sourcev1.Source]{
	{Header: "NAME", Cell: func(s *sourcev1.Source) string { return s.Name }},
	{Header: "KIND", Cell: func(s *sourcev1.Source) string { return kindName(s.Kind) }},
	{Header: "LOCATION", Cell: func(s *sourcev1.Source) string { return s.Location }},
	{Header: "ALLOWS", Cell: func(s *sourcev1.Source) string { return formatAllows(s.Allows) }},
	{Header: "GRANT", Cell: func(s *sourcev1.Source) string { return formatGrant(s) }},
	{Header: "STATUS", Cell: func(s *sourcev1.Source) string { return formatStatus(s) }},
	{Header: "AGE", Cell: func(s *sourcev1.Source) string { return output.FormatAge(s.CreatedAt) }},
	{Header: "TRIGGER", Wide: true, Cell: func(s *sourcev1.Source) string { return output.FormatEnumKebab(s.Trigger) }},
	{Header: "ID", Wide: true, Cell: func(s *sourcev1.Source) string { return s.Id }},
	{Header: "CREATED-BY", Wide: true, Cell: func(s *sourcev1.Source) string { return output.FormatActor(s.CreatedBy) }},
}

// formatAllows says "* (everything)" for the open source, since a bare * in
// a cell reads like a placeholder.
func formatAllows(allows []string) string {
	if len(allows) == 1 && allows[0] == "*" {
		return "* (everything)"
	}
	return strings.Join(allows, ",")
}

// formatGrant is "none" for a public upstream, so an empty cell never reads
// as a missing value.
func formatGrant(s *sourcev1.Source) string {
	switch {
	case s.GetCredentialId() != "":
		return "credential " + s.GetCredentialId()
	case s.GetIntegrationId() != "":
		return "integration " + s.GetIntegrationId()
	default:
		return "none"
	}
}

func formatStatus(s *sourcev1.Source) string {
	if s.Status == sourcev1.Source_STATUS_NEEDS_ATTENTION && s.StatusReason != "" {
		return "needs-attention: " + s.StatusReason
	}
	return output.FormatEnumKebab(s.Status)
}

// kinds maps the flag that names a location to its kind, in the order the
// help lists them.
var kinds = []struct {
	flag string
	kind sourcev1.Source_Kind
	what string
}{
	{"helm", sourcev1.Source_KIND_HELM_REPO, "a Helm repository URL"},
	{"oci", sourcev1.Source_KIND_OCI_REGISTRY, "an OCI registry namespace, as oci://HOST/PATH"},
	{"terraform", sourcev1.Source_KIND_TERRAFORM_REGISTRY, "a Terraform registry namespace, as HOST/NAMESPACE"},
	{"git", sourcev1.Source_KIND_GIT, "a git repository URL"},
}

func kindName(k sourcev1.Source_Kind) string {
	for _, k2 := range kinds {
		if k2.kind == k {
			return k2.flag
		}
	}
	return output.FormatEnumKebab(k)
}
