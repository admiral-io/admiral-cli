package output

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// FormatAge returns a human-readable age string from a protobuf timestamp.
// Example: "5d", "3h", "2m", "10s".
func FormatAge(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "<unknown>"
	}
	return formatDuration(time.Since(ts.AsTime()))
}

// FormatTimestamp returns a formatted timestamp string.
func FormatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "<none>"
	}
	return ts.AsTime().Format(time.RFC3339)
}

// FormatLabels returns a comma-separated key=value string from a label map.
func FormatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		if k == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}

// FormatEnum strips a common prefix from protobuf enum names for display.
// Example: "CLUSTER_HEALTH_STATUS_HEALTHY" with the prefix "CLUSTER_HEALTH_STATUS_" → "Healthy".
func FormatEnum(enumStr, prefix string) string {
	s := strings.TrimPrefix(enumStr, prefix)
	if s == "" || s == "UNSPECIFIED" {
		return "Unknown"
	}
	// Title-case is the result: "HEALTHY" → "Healthy"
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
