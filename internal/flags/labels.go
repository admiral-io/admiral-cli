package flags

import (
	"fmt"
	"maps"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/filter"
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

// Label registers a repeatable --label flag on cmd that collects values into dest.
func Label(cmd *cobra.Command, dest *[]string, usage string) {
	cmd.Flags().Var(&labelFlag{values: dest}, "label", usage)
}

// ParseLabels parses a slice of "key=value" strings into a map.
func ParseLabels(labels []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid label format %q: expected key=value", l)
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}

// ApplyLabelPatch applies kubectl-style label patches over an existing map.
// Each patch is either "key=value" (set/update) or "key-" (remove). The
// returned map is a fresh copy; existing is not mutated.
func ApplyLabelPatch(existing map[string]string, patches []string) (map[string]string, error) {
	out := make(map[string]string, len(existing)+len(patches))
	maps.Copy(out, existing)
	for _, p := range patches {
		if !strings.Contains(p, "=") && strings.HasSuffix(p, "-") {
			key := strings.TrimSuffix(p, "-")
			if key == "" {
				return nil, fmt.Errorf("invalid label %q: key cannot be empty", p)
			}
			delete(out, key)
			continue
		}
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid label format %q: expected key=value or key-", p)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

// LabelFilter converts a slice of "key=value" strings into a filter
// expression for the API (e.g., `labels.region = "us-east-1" AND labels.cloud = "aws"`).
func LabelFilter(labels []string) (string, error) {
	if len(labels) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		kv := strings.SplitN(l, "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			return "", fmt.Errorf("invalid label format %q: expected key=value", l)
		}
		pred, err := filter.Eq("labels."+kv[0], kv[1])
		if err != nil {
			return "", err
		}
		parts = append(parts, pred)
	}
	return filter.And(parts...), nil
}
