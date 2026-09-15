package input

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

func TestParseTime(t *testing.T) {
	now := time.Date(2026, 7, 1, 22, 0, 0, 0, time.UTC)
	got, err := ParseTime("since", "10m", now)
	require.NoError(t, err)
	require.Equal(t, now.Add(-10*time.Minute), got)

	got, err = ParseTime("since", "2026-07-01T20:00:00Z", now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC), got)

	_, err = ParseTime("since", "yesterday", now)
	require.EqualError(t, err, `invalid value "yesterday" for --since: use a duration like 10m or an RFC3339 timestamp`)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}
