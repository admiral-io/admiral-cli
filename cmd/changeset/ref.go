package changeset

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// changeSetRef is a changeset as typed: its ID, or app/env for that
// environment's one open changeset.
type changeSetRef struct {
	id       string
	app, env string
}

func (r changeSetRef) String() string {
	if r.id != "" {
		return r.id
	}
	return r.app + "/" + r.env
}

// parseChangeSet refuses a malformed argument before any sign-in or RPC.
func parseChangeSet(s string) (changeSetRef, error) {
	if app, env, isPath := strings.Cut(s, "/"); isPath {
		if app == "" || env == "" || strings.Contains(env, "/") {
			return changeSetRef{}, cmderr.Usage("invalid environment %q: expected app/env", s)
		}
		return changeSetRef{app: app, env: env}, nil
	}
	if !changeSetIDPattern.MatchString(s) {
		return changeSetRef{}, cmderr.UsageHint("Name a changeset by its ID (cs- and twelve characters) or by its environment as app/env.",
			"invalid changeset %q", s)
	}
	return changeSetRef{id: s}, nil
}

// resolve returns the changeset's ID. An environment resolves only while
// it has exactly one open changeset: with several, picking one would
// apply someone else's change, so the error lists them instead.
func (r changeSetRef) resolve(ctx context.Context, c sdkclient.AdmiralClient) (string, error) {
	if r.id != "" {
		return r.id, nil
	}
	envID, err := resolve.Environment(ctx, c.Environment(), c.Application(), r.app, r.env)
	if err != nil {
		return "", err
	}
	open, err := filter.Eq("status", changesetv1.ChangeSet_STATUS_OPEN.String())
	if err != nil {
		return "", err
	}
	// Two is enough to know it is ambiguous; the list shows a page of them.
	resp, err := c.ChangeSet().ListChangeSets(ctx, &changesetv1.ListChangeSetsRequest{
		EnvironmentId: envID,
		Filter:        open,
		PageSize:      20,
	})
	if err != nil {
		return "", err
	}
	switch css := resp.GetChangeSets(); len(css) {
	case 0:
		return "", cmderr.WithHint(fmt.Errorf("%s has no open changeset", r),
			"Start one: 'admiral changeset create "+r.String()+" --title \"…\"'.")
	case 1:
		return css[0].GetId(), nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%s has %d open changesets; name one:", r, len(css))
		if resp.GetNextPageToken() != "" {
			b.Reset()
			fmt.Fprintf(&b, "%s has more than %d open changesets; name one:", r, len(css))
		}
		for _, cs := range css {
			fmt.Fprintf(&b, "\n  %s  %q  by %s", cs.GetId(), cs.GetTitle(), output.FormatActor(cs.GetCreatedBy()))
			if cs.GetBehind() {
				b.WriteString("  (behind)")
			}
		}
		return "", cmderr.WithHint(fmt.Errorf("%s", b.String()),
			"'admiral changeset list --env "+r.String()+" --status open' lists them all.")
	}
}

// dial signs in and resolves ref. The caller closes the client.
func dial(cmd *cobra.Command, opts *client.Options, ref changeSetRef) (sdkclient.AdmiralClient, string, error) {
	c, err := client.CreateClient(cmd.Context(), opts)
	if err != nil {
		return nil, "", err
	}
	csID, err := ref.resolve(cmd.Context(), c)
	if err != nil {
		c.Close() //nolint:errcheck
		return nil, "", err
	}
	return c, csID, nil
}
