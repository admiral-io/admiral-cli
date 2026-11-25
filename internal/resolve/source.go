package resolve

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.admiral.io/cli/internal/cmderr"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// Source resolves a source name or ID to the source. A miss names the ones
// that exist, so a mistyped name is corrected in one step.
func Source(ctx context.Context, c sourcev1.SourceAPIClient, nameOrID string) (*sourcev1.Source, error) {
	list := func(ctx context.Context, f string) ([]*sourcev1.Source, error) {
		resp, err := c.ListSources(ctx, &sourcev1.ListSourcesRequest{Filter: f})
		if err != nil {
			return nil, err
		}

		return resp.Sources, nil
	}
	id, err := byName(ctx, "source", nameOrID, "", "", "", list,
		func(s *sourcev1.Source) string { return s.Name },
		func(s *sourcev1.Source) string { return s.Id },
	)

	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		all, lerr := list(ctx, "")
		if lerr != nil || len(all) == 0 {
			return nil, err
		}

		names := make([]string, 0, len(all))
		for _, s := range all {
			names = append(names, s.Name)
		}

		return nil, cmderr.WithHint(err, fmt.Sprintf("Sources: %s", strings.Join(names, ", ")))
	}

	if err != nil {
		return nil, err
	}

	resp, err := c.GetSource(ctx, &sourcev1.GetSourceRequest{SourceId: id})
	if err != nil {
		return nil, fmt.Errorf("looking up source %q: %w", nameOrID, err)
	}

	return resp.Source, nil
}
