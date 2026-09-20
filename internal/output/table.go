package output

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	commonv1 "buf.build/gen/go/admiral/common/protocolbuffers/go/admiral/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Placeholders for cells with nothing to show. Angle brackets make them
// unambiguous (a description could legitimately be "-") while keeping the
// cell non-blank so awk column counting still works (kubectl).
const (
	// None marks a field that is absent or empty.
	None = "<none>"
	// Unknown marks a field the server has not determined yet.
	Unknown = "<unknown>"
)

// FormatAge returns the time since ts in kubectl's AGE form: "13s", "5m12s",
// "2h", "5d3h", "41d", "2y". See HumanDuration.
func FormatAge(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return None
	}
	return HumanDuration(time.Since(ts.AsTime()))
}

// FormatTimestamp renders ts as RFC3339 in UTC, for -o wide columns and log
// lines.
func FormatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return None
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}

// FormatDescribeTime renders ts as an absolute local time for describe
// views, the way kubectl does: "Wed, 01 Jul 2026 18:13:49 -0400".
func FormatDescribeTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return None
	}
	return ts.AsTime().Local().Format(DescribeTimeLayout)
}

// DescribeTimeLayout is the absolute local timestamp format used in describe
// and status output.
const DescribeTimeLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

// FormatElapsed renders a duration between two timestamps as Go's
// Duration.String() truncated to seconds: "2m13s", "47s", "1h4m2s".
func FormatElapsed(start, end *timestamppb.Timestamp) string {
	if start == nil {
		return None
	}
	var e time.Time
	if end == nil {
		e = time.Now()
	} else {
		e = end.AsTime()
	}
	d := e.Sub(start.AsTime())
	if d < 0 {
		d = 0
	}
	return d.Truncate(time.Second).String()
}

// HumanDuration is kubectl's age formatter, verbatim from
// k8s.io/apimachinery/pkg/util/duration: at most two units, and the second
// unit is dropped once the first is large enough that it no longer matters.
func HumanDuration(d time.Duration) string {
	// Up to 2s of clock skew reads as "now".
	if seconds := int(d.Seconds()); seconds < -1 {
		return "<invalid>"
	} else if seconds < 0 {
		return "0s"
	} else if seconds < 60*2 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := int(d / time.Minute)
	if minutes < 10 {
		s := int(d/time.Second) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, s)
	} else if minutes < 60*3 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d / time.Hour)
	if hours < 8 {
		m := int(d/time.Minute) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, m)
	} else if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	} else if hours < 24*8 {
		h := hours % 24
		if h == 0 {
			return fmt.Sprintf("%dd", hours/24)
		}
		return fmt.Sprintf("%dd%dh", hours/24, h)
	} else if hours < 24*365*2 {
		return fmt.Sprintf("%dd", hours/24)
	} else if hours < 24*365*8 {
		dy := hours / 24 % 365
		if dy == 0 {
			return fmt.Sprintf("%dy", hours/24/365)
		}
		return fmt.Sprintf("%dy%dd", hours/24/365, dy)
	}
	return fmt.Sprintf("%dy", hours/24/365)
}

// FormatLabels returns a comma-separated key=value string from a label map,
// keys sorted so output is stable across runs.
func FormatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return None
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		if k == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	if len(parts) == 0 {
		return None
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

// LabelLines renders a label map as sorted "k=v" lines for Describe.Fields.
func LabelLines(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+labels[k])
	}
	return out
}

// FormatActor renders an ActorRef for table/detail output, preferring
// display name, then email, then ID.
func FormatActor(a *commonv1.ActorRef) string {
	if a == nil {
		return None
	}
	if a.DisplayName != "" {
		return a.DisplayName
	}
	if a.Email != "" {
		return a.Email
	}
	if a.Id != "" {
		return a.Id
	}
	return None
}

// Truncate shortens s to width runes, ending in a single "…" when cut.
// Widths too small to hold the ellipsis just cut the string.
func Truncate(s string, width int) string {
	if width < 0 {
		width = 0
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

// OrEmpty returns None when s is empty (or whitespace), otherwise s. Use it
// for free-text fields (description, message) so an empty cell renders as
// <none> instead of blank space that misaligns columns.
func OrEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return None
	}
	return s
}

// Tildify shortens a path under $HOME to ~/... for display.
func Tildify(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}
