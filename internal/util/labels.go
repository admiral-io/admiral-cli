package util

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// labelFlag is a pflag.Value that behaves like StringArrayVar but displays
// "string" instead of "stringArray" in help text.
type labelFlag struct {
	values *[]string
}

func (f *labelFlag) String() string {
	if f.values == nil || len(*f.values) == 0 {
		return ""
	}
	return strings.Join(*f.values, ", ")
}

func (f *labelFlag) Set(val string) error {
	*f.values = append(*f.values, val)
	return nil
}

func (f *labelFlag) Type() string { return "string" }

// AddLabelFlag registers a repeatable --label flag on cmd that collects values into dest.
func AddLabelFlag(cmd *cobra.Command, dest *[]string, usage string) {
	cmd.Flags().Var(&labelFlag{values: dest}, "label", usage)
}

// ParseLabels parses a slice of "key=value" strings into a map.
func ParseLabels(labels []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label format %q: expected key=value", l)
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}

// BuildLabelFilter converts a slice of "key=value" strings into a filter
// expression for the API (e.g., `labels.region = "us-east-1" AND labels.cloud = "aws"`).
func BuildLabelFilter(labels []string) (string, error) {
	if len(labels) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		kv := strings.SplitN(l, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("invalid label format %q: expected key=value", l)
		}
		parts = append(parts, fmt.Sprintf("field['labels.%s'] = '%s'", kv[0], kv[1]))
	}
	return strings.Join(parts, " AND "), nil
}
