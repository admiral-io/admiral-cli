package output

import "fmt"

// Format represents the output format for CLI commands.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatWide  Format = "wide"
)

// ParseFormat validates and returns a Format from a string.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatTable, FormatJSON, FormatYAML, FormatWide:
		return Format(s), nil
	default:
		return "", fmt.Errorf("invalid output format %q: must be one of table, json, yaml, wide", s)
	}
}

// String returns the string representation of the Format.
func (f Format) String() string {
	return string(f)
}
