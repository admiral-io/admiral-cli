package changeset

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// recentRunsLimit caps the runs describe lists.
const recentRunsLimit = 5

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "describe <changeset>",
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		Short:             "Show everything about a changeset",
		Long: `Render the operator view of a changeset: where it goes, its status and
head revision, each component it changes with the change in brief, and its
recent runs, with the command to run next. A section the active credential
cannot read is shown as unavailable.

describe is a human view. Use 'changeset get -o json' for the raw record and
'changeset diff' for every value.`,
		Example: `  admiral changeset describe cs-7f2a1c9d0e3b

  # The open changeset of an environment
  admiral changeset describe shop/prod`,
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			ctx := cmd.Context()
			resp, err := c.ChangeSet().GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
			if err != nil {
				return err
			}
			cs := resp.ChangeSet

			// Each section keeps its own error, so a read the credential
			// may not make renders as unavailable without failing the rest.
			var sec csSections
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				sec.diff, sec.diffErr = c.ChangeSet().DiffChangeSet(gctx, &changesetv1.DiffChangeSetRequest{ChangeSetId: csID})
				return nil
			})
			g.Go(func() error {
				sec.runs, sec.moreRuns, sec.runsErr = listChangeSetRuns(gctx, c, csID)
				return nil
			})
			g.Go(func() error {
				sec.where = environmentPath(gctx, c, cs)
				return nil
			})
			_ = g.Wait() // nothing returns an error

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(describeChangeSet(cs, sec), "admiral changeset get "+cs.Id)
		},
	}
	return cmd
}

// csSections holds the reads describe assembles around the changeset.
type csSections struct {
	// where is app/env, or the environment's ID when the names are unreadable.
	where    string
	diff     *changesetv1.DiffChangeSetResponse
	diffErr  error
	runs     []*runv1.Run
	moreRuns bool
	runsErr  error
}

func listChangeSetRuns(ctx context.Context, c sdkclient.AdmiralClient, csID string) ([]*runv1.Run, bool, error) {
	f, err := filter.Eq("change_set_id", csID)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.Run().ListRuns(ctx, &runv1.ListRunsRequest{Filter: f, PageSize: recentRunsLimit})
	if err != nil {
		return nil, false, err
	}
	return resp.Runs, resp.NextPageToken != "", nil
}

// environmentPath names where the changeset goes as app/env, as the commands
// that take an environment write it.
func environmentPath(ctx context.Context, c sdkclient.AdmiralClient, cs *changesetv1.ChangeSet) string {
	env, err := c.Environment().GetEnvironment(ctx, &environmentv1.GetEnvironmentRequest{EnvironmentId: cs.EnvironmentId})
	if err != nil {
		return cs.EnvironmentId
	}
	app, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: cs.ApplicationId})
	if err != nil {
		return cs.EnvironmentId
	}
	return app.GetApplication().GetName() + "/" + env.GetEnvironment().GetName()
}

func describeChangeSet(cs *changesetv1.ChangeSet, sec csSections) *output.Describe {
	d := output.NewDescribe()
	d.Field("Title", cs.Title)
	d.Field("ID", cs.Id)
	d.Field("Environment", sec.where)
	d.Field("Description", cs.Description)
	d.Field("Status", output.FormatEnum(cs.Status))
	d.Field("Revision", strconv.Itoa(int(cs.HeadRevision)))
	if cs.Behind {
		d.Field("Behind", "the environment changed since this revision was made")
	}
	if cs.RevertsChangeSetId != "" {
		d.Field("Reverts", cs.RevertsChangeSetId)
	}
	var latest *runv1.Run
	if len(sec.runs) > 0 {
		latest = sec.runs[0]
	}
	switch {
	case latest != nil:
		d.Field("Last Run", fmt.Sprintf("%s %s %s ago", latest.Id, output.FormatEnum(latest.Status), output.FormatAge(latest.CreatedAt)))
	case sec.runsErr != nil && cs.LatestRunId != "":
		d.Field("Last Run", cs.LatestRunId)
	default:
		d.Field("Last Run", "")
	}
	d.Field("Created", output.FormatDescribeTime(cs.CreatedAt))
	d.Field("Created By", output.FormatActorWithID(cs.CreatedBy))

	if sec.diffErr != nil {
		d.Unavailable("Components", cmderr.Format(sec.diffErr))
	} else {
		d.Section("Components", func(b *output.Block) {
			comps := sec.diff.GetComponents()
			if len(comps) == 0 {
				b.Field("Count", "0")
				return
			}
			rows := make([][]string, 0, len(comps))
			for _, c := range comps {
				rows = append(rows, []string{c.Component, output.FormatEnumKebab(c.Action),
					strings.TrimSpace(versionChange(c.OldVersion, c.NewVersion)), changesInBrief(c)})
			}
			b.Table([]string{"Name", "Action", "Version", "Changes"}, rows)
		})
	}

	if sec.runsErr != nil {
		d.Unavailable("Runs", cmderr.Format(sec.runsErr))
	} else if len(sec.runs) > 0 {
		rows := make([][]string, 0, len(sec.runs))
		for _, r := range sec.runs {
			rows = append(rows, []string{r.Id, output.FormatEnum(r.Status), output.FormatActor(r.CreatedBy), output.FormatAge(r.CreatedAt)})
		}
		d.Table("Runs", []string{"ID", "Status", "Created By", "Age"}, rows)
	}

	for _, h := range nextSteps(cs, latest, sec) {
		d.Hint(h)
	}
	if status.Code(sec.diffErr) == codes.PermissionDenied || status.Code(sec.runsErr) == codes.PermissionDenied {
		d.Hint(cmderr.ScopeHint)
	}
	return d
}

// changesInBrief is a component's one change written out, or a count of its
// values, with its violations counted.
func changesInBrief(c *changesetv1.ComponentDiff) string {
	var leaves []output.Leaf
	for _, pd := range c.Paths {
		leaves = append(leaves, output.Leaves(pd.DisplayPath, pd.BeforePresent, pd.BeforeJson, pd.AfterPresent, pd.AfterJson)...)
	}
	var out string
	switch len(leaves) {
	case 0:
	case 1:
		l := leaves[0]
		out = fmt.Sprintf("%s: %s -> %s", l.Path, l.Before, l.After)
		if len(out) > 60 {
			out = "1 value"
		}
	default:
		out = fmt.Sprintf("%d values", len(leaves))
	}
	if n := len(c.Violations); n > 0 {
		v := fmt.Sprintf("%d violations", n)
		if n == 1 {
			v = "1 violation"
		}
		if out != "" {
			out += ", "
		}
		out += v
	}
	return out
}

// nextSteps are the commands to run next, from where the changeset stands.
func nextSteps(cs *changesetv1.ChangeSet, latest *runv1.Run, sec csSections) []string {
	var hints []string
	if n := len(sec.diff.GetComponents()); n > 0 {
		hints = append(hints, "To see every value, run: admiral changeset diff "+cs.Id)
	}
	if cs.Status != changesetv1.ChangeSet_STATUS_OPEN {
		return hints
	}
	switch {
	case latest == nil && cs.LatestRunId == "":
		hints = append(hints, "To plan it, run: admiral changeset plan "+cs.Id)
	case latest == nil:
	case latest.Status == runv1.Run_STATUS_PLANNED:
		hints = append(hints, "To apply it, run: admiral changeset apply "+cs.Id)
	case latest.Status == runv1.Run_STATUS_FAILED:
		hints = append(hints, "To see why the last run failed, run: admiral run get "+latest.Id,
			"To plan it again, run: admiral changeset plan "+cs.Id)
	}
	if sec.moreRuns {
		hints = append(hints, fmt.Sprintf("Showing the %d most recent runs. To list them all, run: admiral run list --changeset %s",
			recentRunsLimit, cs.Id))
	}
	return hints
}
