package component

import (
	"strconv"
	"strings"

	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// componentTable is the narrow row shared by list, get and the echo after
// publish.
var componentTable = output.Table[*registryv1.Component]{
	{Header: "NAME", Cell: func(c *registryv1.Component) string { return c.Name }},
	{Header: "TAGS", Cell: func(c *registryv1.Component) string { return formatTags(c.Tags) }},
	{Header: "STATUS", Cell: func(c *registryv1.Component) string { return output.FormatEnumKebab(c.Status) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(c *registryv1.Component) string { return c.Description }},
	{Header: "LABELS", Cell: func(c *registryv1.Component) string { return output.FormatLabels(c.Labels) }},
	{Header: "AGE", Cell: func(c *registryv1.Component) string { return output.FormatAge(c.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(c *registryv1.Component) string { return c.Id }},
	{Header: "CREATED-BY", Wide: true, Cell: func(c *registryv1.Component) string { return output.FormatActor(c.CreatedBy) }},
}

// versionTable is what a publish answers with.
var versionTable = output.Table[*registryv1.Version]{
	{Header: "DIGEST", Cell: func(v *registryv1.Version) string { return shortDigest(v.Digest) }},
	{Header: "KIND", Cell: func(v *registryv1.Version) string { return output.FormatEnumKebab(v.Kind) }},
	{Header: "TAGS", Cell: func(v *registryv1.Version) string { return strings.Join(v.Tags, ",") }},
	{Header: "STATUS", Cell: func(v *registryv1.Version) string { return output.FormatEnumKebab(v.Status) }},
	{Header: "FINDINGS", Cell: func(v *registryv1.Version) string { return formatFindingCount(v.Findings) }},
	{Header: "AGE", Cell: func(v *registryv1.Version) string { return output.FormatAge(v.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(v *registryv1.Version) string { return v.Id }},
	{Header: "FULL-DIGEST", Wide: true, Cell: func(v *registryv1.Version) string { return v.Digest }},
	{Header: "SIZE", Wide: true, Cell: func(v *registryv1.Version) string { return formatSize(v.SizeBytes) }},
}

func formatTags(tags []*registryv1.Tag) string {
	if len(tags) == 0 {
		return "" // the table renders the empty cell as <none>
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

// shortDigest is the first twelve hex characters, the docker convention:
// enough to tell versions apart in a table, and -o wide has the whole thing.
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
		registryv1.FindingSeverity_FINDING_SEVERITY_CRITICAL, registryv1.FindingSeverity_FINDING_SEVERITY_HIGH,
		registryv1.FindingSeverity_FINDING_SEVERITY_MEDIUM, registryv1.FindingSeverity_FINDING_SEVERITY_LOW, registryv1.FindingSeverity_FINDING_SEVERITY_INFO,
	} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, output.FormatEnumKebab(sev)+":"+strconv.Itoa(n))
		}
	}
	return strings.Join(parts, " ")
}

// sizeUnits are the binary prefixes formatSize can print; anything past the
// last one is expressed in it rather than indexing off the end.
var sizeUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 1
	for m := n / unit; m >= unit && exp < len(sizeUnits)-1; m /= unit {
		div *= unit
		exp++
	}
	v := strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64)
	return strings.TrimSuffix(strings.TrimSuffix(v, "0"), ".") + " " + sizeUnits[exp]
}
