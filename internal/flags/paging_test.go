package flags

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// pagesOf is a server with three pages of one item each.
func pagesOf(t *testing.T, calls *[]string) func(string) ([]string, string, error) {
	t.Helper()
	pages := map[string]struct {
		items []string
		next  string
	}{
		"":   {[]string{"a"}, "p2"},
		"p2": {[]string{"b"}, "p3"},
		"p3": {[]string{"c"}, ""},
	}
	return func(token string) ([]string, string, error) {
		*calls = append(*calls, token)
		p, ok := pages[token]
		if !ok {
			return nil, "", errors.New("no such page")
		}
		return p.items, p.next, nil
	}
}

func TestPages_OnePage(t *testing.T) {
	var calls []string
	items, next, err := Pages(PagingOptions{PageToken: "p2"}, pagesOf(t, &calls))
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, items)
	require.Equal(t, "p3", next, "the caller is told where the next page starts")
	require.Equal(t, []string{"p2"}, calls)
}

func TestPages_All(t *testing.T) {
	var calls []string
	items, next, err := Pages(PagingOptions{All: true}, pagesOf(t, &calls))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, items)
	require.Empty(t, next, "nothing is left over under --all")
	require.Equal(t, []string{"", "p2", "p3"}, calls)
}

func TestPages_AllStopsOnARepeatedToken(t *testing.T) {
	_, _, err := Pages(PagingOptions{All: true}, func(string) ([]string, string, error) {
		return []string{"x"}, "same", nil
	})
	require.ErrorContains(t, err, "same page token twice")
}

func TestPages_ErrorMidWay(t *testing.T) {
	calls := 0
	_, _, err := Pages(PagingOptions{All: true}, func(string) ([]string, string, error) {
		calls++
		if calls == 2 {
			return nil, "", errors.New("boom")
		}
		return []string{"x"}, "next", nil
	})
	require.EqualError(t, err, "boom")
}

func TestPaging_AllExcludesPageToken(t *testing.T) {
	newCmd := func(o *PagingOptions) *cobra.Command {
		cmd := &cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }}
		Paging(cmd, o)
		return cmd
	}

	var o PagingOptions
	cmd := newCmd(&o)
	cmd.SetArgs([]string{"--all", "--page-token", "x"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "if any flags in the group [all page-token] are set")

	cmd = newCmd(&o)
	cmd.SetArgs([]string{"--all", "--page-size", "10"})
	require.NoError(t, cmd.Execute())
	require.True(t, o.All)
	require.EqualValues(t, 10, o.PageSize)
}
