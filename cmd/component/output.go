package component

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// componentTable is the narrow row shared by list, get and the echo after
// publish.
var componentTable = output.Table[*registryv1.Component]{
	{Header: "NAME", Cell: func(c *registryv1.Component) string { return c.Name }},
	{Header: "TAGS", Cell: func(c *registryv1.Component) string { return formatTags(c.Tags) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(c *registryv1.Component) string { return c.Description }},
	{Header: "LABELS", Cell: func(c *registryv1.Component) string { return output.FormatLabels(c.Labels) }},
	{Header: "AGE", Cell: func(c *registryv1.Component) string { return output.FormatAge(c.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(c *registryv1.Component) string { return c.Id }},
	{Header: "CREATED-BY", Wide: true, Cell: func(c *registryv1.Component) string { return output.FormatActor(c.CreatedBy) }},
}

// revisionTable is what a publish answers with.
var revisionTable = output.Table[*registryv1.Revision]{
	{Header: "DIGEST", Cell: func(r *registryv1.Revision) string { return shortDigest(r.Digest) }},
	{Header: "KIND", Cell: func(r *registryv1.Revision) string { return output.FormatEnumKebab(r.Kind) }},
	{Header: "TAGS", Cell: func(r *registryv1.Revision) string { return strings.Join(r.Tags, ",") }},
	{Header: "STATUS", Cell: func(r *registryv1.Revision) string { return output.FormatEnumKebab(r.Status) }},
	{Header: "FINDINGS", Cell: func(r *registryv1.Revision) string { return formatFindingCount(r.Findings) }},
	{Header: "AGE", Cell: func(r *registryv1.Revision) string { return output.FormatAge(r.CreatedAt) }},
	{Header: "FULL-DIGEST", Wide: true, Cell: func(r *registryv1.Revision) string { return r.Digest }},
	{Header: "SIZE", Wide: true, Cell: func(r *registryv1.Revision) string { return formatSize(r.SizeBytes) }},
}

func formatTags(tags []*registryv1.Tag) string {
	if len(tags) == 0 {
		return "-"
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

// shortDigest is the first twelve hex characters, the docker convention:
// enough to tell revisions apart in a table, and -o wide has the whole thing.
func shortDigest(d string) string {
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return hex
}

func formatFindingCount(fs []*registryv1.Finding) string {
	if len(fs) == 0 {
		return "0"
	}
	counts := map[registryv1.FindingSeverity]int{}
	for _, f := range fs {
		counts[f.Severity]++
	}
	var parts []string
	for _, sev := range []registryv1.FindingSeverity{
		registryv1.FindingSeverity_CRITICAL, registryv1.FindingSeverity_HIGH,
		registryv1.FindingSeverity_MEDIUM, registryv1.FindingSeverity_LOW, registryv1.FindingSeverity_INFO,
	} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, strings.ToLower(sev.String())+":"+itoa(n))
		}
	}
	return strings.Join(parts, " ")
}

func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return itoa(int(n)) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(strings.TrimSuffix(ftoa(float64(n)/float64(div)), "0"), ".") + " " + "KMGT"[exp:exp+1] + "iB"
}
