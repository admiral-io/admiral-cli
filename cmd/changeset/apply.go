package changeset

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	runcmd "go.admiral.io/cli/cmd/run"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newApproveCmd(opts *client.Options) *cobra.Command {
	var (
		reason string
		yes    bool
	)
	cmd := &cobra.Command{
		Use:   "approve <changeset>",
		Short: "Approve a changeset's latest plan",
		Long: `Approve the plan of a changeset's latest run. The plan is shown first and
the command asks before approving; --yes skips the question, not the
plan. The approval names the plan's digest, so a newer plan is not
approved by it. The changeset's author cannot approve it.`,
		Example: `  admiral changeset approve shop/prod

  # With a reason, without the question
  admiral changeset approve cs-7f2a1c9d0e3b --reason "reviewed in #412" --yes`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}
			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			cs, r, err := latestPlan(cmd.Context(), c, csID)
			if err != nil {
				return err
			}
			if err := showPlan(cmd.Context(), cmd.ErrOrStderr(), c, cs, r); err != nil {
				return err
			}
			if err := input.Confirm(cmd, yes, "Approve this plan"); err != nil {
				return err
			}
			if _, err := c.Run().ApproveRun(cmd.Context(), &runv1.ApproveRunRequest{
				RunId: r.GetId(), PlanDigest: r.GetPlan().GetPlanDigest(), Reason: reason,
			}); err != nil {
				return runcmd.Refusal(err, "admiral changeset", csID)
			}
			output.Confirmed(cmd.ErrOrStderr(), "changeset", csID, "approved ("+r.GetId()+")")
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded with the approval")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newApplyCmd(opts *client.Options) *cobra.Command {
	var (
		yes bool
		po  planOptions
	)
	cmd := &cobra.Command{
		Use:   "apply <changeset>",
		Short: "Apply a changeset's latest plan",
		Long: `Apply the plan of a changeset's latest run, once its required approvals
are in. The plan is shown first and the command asks before applying;
--yes skips the question and nothing else: approvals, a changeset that has
fallen behind and the environment's lock are checked either way.

The command waits for the apply to end unless --no-wait, and exits 1 when
it failed or applied partway.`,
		Example: `  admiral changeset apply shop/dev

  # From a script
  admiral changeset apply cs-7f2a1c9d0e3b --yes`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			if po.waitTimeout <= 0 {
				return cmderr.Usage("--wait-timeout must be positive")
			}
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}
			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			cs, r, err := latestPlan(cmd.Context(), c, csID)
			if err != nil {
				return err
			}
			if err := showPlan(cmd.Context(), cmd.ErrOrStderr(), c, cs, r); err != nil {
				return err
			}
			if err := input.Confirm(cmd, yes, "Apply this plan"); err != nil {
				return err
			}
			resp, err := c.Run().ApplyRun(cmd.Context(), &runv1.ApplyRunRequest{
				RunId: r.GetId(), PlanDigest: r.GetPlan().GetPlanDigest(),
			})
			if err != nil {
				return runcmd.Refusal(err, "admiral changeset", csID)
			}
			r = resp.GetRun()
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if !po.noWait {
				done, err := runcmd.Wait(cmd.Context(), c.Run(), r, po.waitTimeout, p.Err())
				if err != nil {
					return runcmd.WaitTimeout(err, r)
				}
				r = done
			}
			if err := runcmd.Print(p, r); err != nil {
				return err
			}
			return runcmd.Outcome(r)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	waitFlagsFor(cmd, &po, "the apply")
	return cmd
}

// latestPlan is the changeset and its latest run, which must hold a plan
// waiting to be approved or applied.
func latestPlan(ctx context.Context, c sdkclient.AdmiralClient, csID string) (*changesetv1.ChangeSet, *runv1.Run, error) {
	resp, err := c.ChangeSet().GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
	if err != nil {
		return nil, nil, err
	}
	cs := resp.GetChangeSet()
	if cs.GetLatestRunId() == "" {
		return nil, nil, cmderr.WithHint(fmt.Errorf("%s has not been planned", csID),
			"Run 'admiral changeset plan "+csID+"'.")
	}
	run, err := c.Run().GetRun(ctx, &runv1.GetRunRequest{RunId: cs.GetLatestRunId()})
	if err != nil {
		return nil, nil, err
	}
	r := run.GetRun()
	switch r.GetStatus() {
	case runv1.Run_STATUS_PLANNED:
		return cs, r, nil
	case runv1.Run_STATUS_NO_CHANGES:
		return nil, nil, fmt.Errorf("%s's latest plan (%s) found nothing to change", csID, r.GetId())
	case runv1.Run_STATUS_PREPARING, runv1.Run_STATUS_QUEUED, runv1.Run_STATUS_PLANNING:
		return nil, nil, cmderr.WithHint(fmt.Errorf("%s is still planning (%s)", csID, r.GetId()),
			"Wait for it: 'admiral run get "+r.GetId()+" --wait'.")
	case runv1.Run_STATUS_FAILED:
		return nil, nil, cmderr.WithHint(fmt.Errorf("%s's latest plan failed: %s", csID, r.GetPlan().GetMessage()),
			"Fix the changeset and plan again: 'admiral changeset plan "+csID+"'.")
	default:
		return nil, nil, fmt.Errorf("%s's latest run %s is %s, not waiting on a plan", csID, r.GetId(),
			output.FormatEnumKebab(r.GetStatus()))
	}
}

// showPlan writes what is about to be approved or applied: which
// changeset, whose, and its diff resource by resource. It goes to stderr
// with the question; stdout stays the answer.
func showPlan(ctx context.Context, w io.Writer, c sdkclient.AdmiralClient, cs *changesetv1.ChangeSet, r *runv1.Run) error {
	diff, err := c.Run().GetRunPlanDiff(ctx, &runv1.GetRunPlanDiffRequest{RunId: r.GetId()})
	if err != nil {
		return err
	}
	output.Writef(w, "%s  %q  by %s  (%s)\n", cs.GetId(), cs.GetTitle(), output.FormatActor(cs.GetCreatedBy()), r.GetId())
	runcmd.WriteDiff(w, diff.GetDiff(), r.GetPlan())
	return nil
}
