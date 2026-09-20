package flags

import (
	"errors"

	"github.com/spf13/cobra"
)

// PagingOptions holds the three ways a list can be walked: one page of
// --page-size from --page-token, or every page with --all.
type PagingOptions struct {
	PageSize  int32
	PageToken string
	All       bool
}

// Paging registers --page-size, --page-token and --all on a list command.
// --all and --page-token contradict each other and are refused together.
func Paging(cmd *cobra.Command, o *PagingOptions) {
	cmd.Flags().Int32Var(&o.PageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&o.PageToken, "page-token", "", "pagination token from a previous response")
	cmd.Flags().BoolVar(&o.All, "all", false, "fetch every page (a script then never has to read the next page token)")
	cmd.MarkFlagsMutuallyExclusive("all", "page-token")
}

// Pages fetches one page, or every page under --all, through fetch, which
// requests the page at token and returns its items and the token that
// follows. The returned token is "" when there is nothing more to fetch,
// which under --all is always.
func Pages[T any](o PagingOptions, fetch func(token string) ([]T, string, error)) ([]T, string, error) {
	items, next, err := fetch(o.PageToken)
	if err != nil {
		return nil, "", err
	}
	if !o.All {
		return items, next, nil
	}
	for next != "" {
		more, after, err := fetch(next)
		if err != nil {
			return nil, "", err
		}
		items = append(items, more...)
		if after == next {
			return nil, "", errors.New("the server returned the same page token twice; stopping")
		}
		next = after
	}
	return items, "", nil
}
