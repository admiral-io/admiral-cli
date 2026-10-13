package run

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func runID(s string) (string, error) {
	if !resolve.IsID("run", s) {
		return "", cmderr.Usage("%q is not a run ID (run-xxxxxxxxxxxx)", s)
	}
	return s, nil
}

// waitOptions are --wait and --wait-timeout on a verb that can wait for the
// run to settle.
type waitOptions struct {
	wait    bool
	timeout time.Duration
}

func (w *waitOptions) register(cmd *cobra.Command, def bool, what string) {
	cmd.Flags().BoolVar(&w.wait, "wait", def, "wait until "+what)
	cmd.Flags().DurationVar(&w.timeout, "wait-timeout", DefaultWaitTimeout, "how long to wait; exits 124 when it elapses")
}

// finish prints r, after waiting for it to settle when asked. The exit
// code is the run's outcome.
func (w waitOptions) finish(cmd *cobra.Command, opts *client.Options, c runv1.RunAPIClient, r *runv1.Run) error {
	p := output.NewPrinter(cmd, opts.OutputFormat)
	if w.wait {
		var err error
		if r, err = Wait(cmd.Context(), c, r, w.timeout, p.Err()); err != nil {
			return WaitTimeout(err, r)
		}
	}
	if err := Print(p, r); err != nil {
		return err
	}
	return Outcome(r)
}

func getRun(ctx context.Context, c runv1.RunAPIClient, id string) (*runv1.Run, error) {
	resp, err := c.GetRun(ctx, &runv1.GetRunRequest{RunId: id})
	return resp.GetRun(), err
}

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		env, changeSet, status string
		paging                 flags.PagingOptions
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs, newest first",
		Example: `  admiral run list --env shop/prod
  admiral run list --changeset cs-7f2a1c9d0e3b`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			var filters []string
			if env != "" {
				app, name, err := flags.EnvTarget(cmd, "", env)
				if err != nil {
					return err
				}
				id, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, name)
				if err != nil {
					return err
				}
				filters = append(filters, fmt.Sprintf("field['environment_id'] = '%s'", id))
			}
			if changeSet != "" {
				filters = append(filters, fmt.Sprintf("field['change_set_id'] = '%s'", changeSet))
			}
			if status != "" {
				filters = append(filters, fmt.Sprintf("field['status'] = 'STATUS_%s'", strings.ToUpper(strings.ReplaceAll(status, "-", "_"))))
			}
			runs, next, err := flags.Pages(paging, func(token string) ([]*runv1.Run, string, error) {
				resp, err := c.Run().ListRuns(cmd.Context(), &runv1.ListRunsRequest{
					Filter: strings.Join(filters, " AND "), PageSize: paging.PageSize, PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.GetRuns(), resp.GetNextPageToken(), nil
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind: "runs", Items: output.Messages(runs), NextPageToken: next,
				Name: func(i int) string { return runs[i].GetId() },
			}, runTable.Render(p, runs...))
		},
	}
	cmd.Flags().StringVar(&env, "env", "", "runs of one environment, as app/env")
	cmd.Flags().StringVar(&changeSet, "changeset", "", "runs of one changeset")
	cmd.Flags().StringVar(&status, "status", "", "runs in one status, e.g. planned or failed")
	flags.Paging(cmd, &paging)
	return cmd
}

func newGetCmd(opts *client.Options) *cobra.Command {
	var w waitOptions
	cmd := &cobra.Command{
		Use:   "get <run>",
		Short: "Get a run: its plan, approvals and apply",
		Long: `Get a run. With --wait, poll until it settles: planned and waiting for
people, or ended. Exits 1 when it failed, applied partway or was superseded.`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			r, err := getRun(cmd.Context(), c.Run(), id)
			if err != nil {
				return err
			}
			if !w.wait {
				return Print(output.NewPrinter(cmd, opts.OutputFormat), r)
			}
			return w.finish(cmd, opts, c.Run(), r)
		},
	}
	w.register(cmd, false, "the run settles")
	return cmd
}

func newApproveCmd(opts *client.Options) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "approve <run>",
		Short: "Approve a run's plan",
		Long: `Approve the plan a run holds now. The approval names the plan's digest,
so a newer plan of the same run is not approved by it.`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			r, err := getRun(cmd.Context(), c.Run(), id)
			if err != nil {
				return err
			}
			if _, err := c.Run().ApproveRun(cmd.Context(), &runv1.ApproveRunRequest{
				RunId: id, PlanDigest: r.GetPlan().GetPlanDigest(), Reason: reason,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run %s approved\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded with the approval")
	return cmd
}

func newApplyCmd(opts *client.Options) *cobra.Command {
	var w waitOptions
	cmd := &cobra.Command{
		Use:   "apply <run>",
		Short: "Apply a planned run",
		Long: `Apply the plan a run holds now, once its required approvals are in. The
command waits for the apply to end unless --wait=false; exits 1 when it
failed or applied partway.`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			r, err := getRun(cmd.Context(), c.Run(), id)
			if err != nil {
				return err
			}
			resp, err := c.Run().ApplyRun(cmd.Context(), &runv1.ApplyRunRequest{RunId: id, PlanDigest: r.GetPlan().GetPlanDigest()})
			if err != nil {
				return err
			}
			return w.finish(cmd, opts, c.Run(), resp.GetRun())
		},
	}
	w.register(cmd, true, "the apply ends")
	return cmd
}

func newAcceptCmd(opts *client.Options) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "accept <run>",
		Short: "Accept a partial apply as the environment's state",
		Long: `Accept a run that applied partway. The environment is released, with the
components that applied recorded as live and the rest as they were.`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			resp, err := c.Run().AcceptRun(cmd.Context(), &runv1.AcceptRunRequest{RunId: id, Reason: reason})
			if err != nil {
				return err
			}
			return Print(output.NewPrinter(cmd, opts.OutputFormat), resp.GetRun())
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded with the acceptance")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func newCancelCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <run>",
		Short: "Cancel a run that has not started applying",
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			if _, err := c.Run().CancelRun(cmd.Context(), &runv1.CancelRunRequest{RunId: id}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run %s canceled\n", id)
			return nil
		},
	}
}

func newDiffCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <run>",
		Short: "Show what a run's plan changes, resource by resource",
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			resp, err := c.Run().GetRunPlanDiff(cmd.Context(), &runv1.GetRunPlanDiffRequest{RunId: id})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if !opts.OutputFormat.IsTable() {
				return p.PrintResource(resp, nil)
			}
			for _, rd := range resp.GetDiff().GetResources() {
				output.Writef(p.Out(), "# %s: %s\n%s\n", rd.GetComponent(), resourceName(rd.GetResource()), rd.GetDiff())
			}
			return nil
		},
	}
}

func resourceName(r *agentv1.ResourceRef) string {
	name := r.GetKind() + "/" + r.GetName()
	if r.GetNamespace() != "" {
		name = r.GetNamespace() + "/" + name
	}
	return name
}

func newLogsCmd(opts *client.Options) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <run>",
		Short: "Print the output of a run's jobs",
		Long: `Print what the agents wrote while planning and applying, job by job.
With --follow, keep printing until every job has ended.`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			r, err := getRun(cmd.Context(), c.Run(), id)
			if err != nil {
				return err
			}
			jobs := append(r.GetPlan().GetJobs(), r.GetApply().GetJobs()...)
			for _, j := range jobs {
				output.Writef(cmd.OutOrStdout(), "==> job %s (%s)\n", j.GetJobId(), strings.Join(j.GetComponents(), ", "))
				if err := streamLog(cmd.Context(), c.Job(), j.GetJobId(), follow, cmd.OutOrStdout()); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing until every job ends")
	return cmd
}

func streamLog(ctx context.Context, c agentv1.JobAPIClient, jobID string, follow bool, w io.Writer) error {
	var offset int64
	for {
		resp, err := c.GetJobLog(ctx, &agentv1.GetJobLogRequest{JobId: jobID, Offset: offset})
		if err != nil {
			return err
		}
		if _, err := w.Write(resp.GetChunk()); err != nil {
			return err
		}
		if resp.GetTruncated() {
			output.Writef(w, "[log truncated]\n")
		}
		offset = resp.GetNextOffset()
		if resp.GetComplete() || (!follow && len(resp.GetChunk()) == 0) {
			return nil
		}
		if len(resp.GetChunk()) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
}
