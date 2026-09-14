package input

import (
	"time"

	"go.admiral.io/cli/internal/cmderr"
)

// ParseTime reads the value of a --since/--until style flag: a Go duration
// ("10m", "2h30m") is relative to now, anything else must be RFC3339. The
// flag name is only used in the error.
func ParseTime(flag, value string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(value); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Time{}, cmderr.Usage("invalid value %q for --%s: use a duration like 10m or an RFC3339 timestamp", value, flag)
}
