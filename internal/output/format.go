package output

import "fmt"

// Format represents the output format for CLI commands.
type Format string

const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	// FormatName prints one bare identifier per line (a name, or an ID for
	// resources addressed by ID) and nothing else: the xargs contract.
	FormatName Format = "name"
)

// Formats lists every accepted value, in the order help text shows them.
var Formats = []Format{FormatTable, FormatWide, FormatJSON, FormatYAML, FormatName}

// ParseFormat validates and returns a Format from a string.
func ParseFormat(s string) (Format, error) {
	for _, f := range Formats {
		if Format(s) == f {
			return f, nil
		}
	}
	return "", fmt.Errorf("invalid output format %q: must be one of table, wide, json, yaml, name", s)
}

// String returns the string representation of the Format.
func (f Format) String() string {
	return string(f)
}

// IsTable reports whether f renders a human table (table or wide).
func (f Format) IsTable() bool {
	return f == FormatTable || f == FormatWide
}

// IsMachine reports whether f is a machine-readable format (json or yaml).
func (f Format) IsMachine() bool {
	return f == FormatJSON || f == FormatYAML
}
