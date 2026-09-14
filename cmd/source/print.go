package source

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// shortResolved renders a resolved handle compactly: a 40-char git SHA or an
// OCI digest is truncated to its first 12 chars; shorter values are shown
// whole. Empty resolves render as the empty marker.
func shortResolved(resolved string) string {
	if resolved == "" {
		return output.None
	}
	trimmed := strings.TrimPrefix(resolved, "sha256:")
	if len(trimmed) > 12 {
		return trimmed[:12]
	}
	return trimmed
}

// credentialDisplay renders the Source's credential as a name (preferred) or
// truncated UUID, falling back to None when the source is public
// (credential_id unset). Mirrors orID with optional-string semantics.
func credentialDisplay(s *sourcev1.Source) string {
	if s.CredentialName != "" {
		return s.CredentialName
	}
	if s.CredentialId != nil && *s.CredentialId != "" {
		id := *s.CredentialId
		if len(id) >= 8 {
			return id[:8]
		}
		return id
	}
	return output.None
}

// printSourceRow writes a single-row summary (header + values) to w.
// sourceTable is the single column definition shared by list, get and the
// create/update echo.
var sourceTable = output.Table[*sourcev1.Source]{
	{Header: "NAME", Cell: func(s *sourcev1.Source) string { return s.Name }},
	{Header: "TYPE", Cell: func(s *sourcev1.Source) string { return output.FormatEnumKebab(s.Type) }},
	{Header: "URL", Cell: func(s *sourcev1.Source) string { return s.Url }},
	{Header: "CREDENTIAL", Cell: credentialDisplay},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(s *sourcev1.Source) string { return s.Description }},
	{Header: "LABELS", Cell: func(s *sourcev1.Source) string { return output.FormatLabels(s.Labels) }},
	{Header: "LAST TEST", Cell: func(s *sourcev1.Source) string { return output.FormatEnum(s.LastTestStatus) }},
	{Header: "AGE", Cell: func(s *sourcev1.Source) string { return output.FormatAge(s.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(s *sourcev1.Source) string { return s.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(s *sourcev1.Source) string { return output.FormatActor(s.CreatedBy) }},
}

// versionTable renders the versions a source publishes (`source versions`).
var versionTable = output.Table[*sourcev1.SourceVersion]{
	{Header: "VERSION", Cell: func(v *sourcev1.SourceVersion) string { return v.Version }},
	{Header: "KIND", Cell: func(v *sourcev1.SourceVersion) string { return output.FormatEnumKebab(v.Kind) }},
	{Header: "RESOLVED", Cell: func(v *sourcev1.SourceVersion) string { return shortResolved(v.Resolved) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(v *sourcev1.SourceVersion) string { return v.Description }},
	{Header: "AGE", Cell: func(v *sourcev1.SourceVersion) string { return output.FormatAge(v.PublishedAt) }},
	{Header: "PUBLISHED", Wide: true, Cell: func(v *sourcev1.SourceVersion) string { return output.FormatTimestamp(v.PublishedAt) }},
}

// testResultTable renders the outcome of `source test`.
var testResultTable = output.Table[*sourcev1.TestSourceResponse]{
	{Header: "STATUS", Cell: func(r *sourcev1.TestSourceResponse) string { return output.FormatEnum(r.Status) }},
	{Header: "ERROR", Cell: func(r *sourcev1.TestSourceResponse) string { return r.Error }},
}
