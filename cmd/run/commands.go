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
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
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
	// follow keeps waiting while the agent enrolls: the user asked to follow
	// this run, often right after installing its agent.
	follow bool
}

func (w *waitOptions) register(cmd *cobra.Command, def bool, what string) {
	cmd.Flags().BoolVar(&w.wait, "wait", def, "wait until "+what)
	cmd.Flags().DurationVar(&w.timeout, "wait-timeout", DefaultWaitTimeout, "how long to wait; exits 124 when it elapses")
}

// finish prints r, after waiting for it to settle when asked. The exit
// code is the run's outcome.
func (w waitOptions) finish(cmd *cobra.Command, opts *client.Options, c sdkclient.AdmiralClient, r *runv1.Run) error {
	p := output.NewPrinter(cmd, opts.OutputFormat)
	if w.wait {
		var err error
		if r, err = Wait(cmd.Context(), c.Run(), r, w.timeout, !w.follow, p.Err()); err != nil {
			return WaitError(err, r)
		}
	}
	if err := printTitled(cmd, p, c, r); err != nil {
		return err
	}
	return Outcome(r)
}

// printTitled is Print with the title of the run's changeset read first.
func printTitled(cmd *cobra.Command, p *output.Printer, c sdkclient.AdmiralClient, r *runv1.Run) error {
	if p.Format != output.FormatTable && p.Format != output.FormatWide {
		return Print(p, r, "")
	}
	return Print(p, r, ChangeSetTitles(cmd.Context(), c.ChangeSet(), r.GetChangeSetId())[r.GetChangeSetId()])
}

// ChangeSetTitles reads each distinct changeset for its title. One that
// cannot be read is left out, so the caller shows its id alone.
func ChangeSetTitles(ctx context.Context, c changesetv1.ChangeSetAPIClient, ids ...string) map[string]string {
	titles := make(map[string]string, len(ids))
	for _, id := range ids {
		if _, seen := titles[id]; seen || id == "" {
			continue
		}
		resp, err := c.GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: id})
		if err != nil {
			continue
		}
		titles[id] = resp.GetChangeSet().GetTitle()
	}
	return titles
}

func getRun(ctx context.Context, c runv1.RunAPIClient, id string) (*runv1.Run, error) {
	resp, err := c.GetRun(ctx, &runv1.GetRunRequest{RunId: id})
	return resp.GetRun(), err
}

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		env, changeSet string
		paging         flags.PagingOptions
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
	complete.Flag(cmd, "env", complete.Envs(opts))
	cmd.Flags().StringVar(&changeSet, "changeset", "", "runs of one changeset")
	flags.Paging(cmd, &paging)
	return cmd
}

func newGetCmd(opts *client.Options) *cobra.Command {
	w := waitOptions{follow: true}
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
				return printTitled(cmd, output.NewPrinter(cmd, opts.OutputFormat), c, r)
			}
			return w.finish(cmd, opts, c, r)
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
				return Refusal(err, "admiral run", id)
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
				return Refusal(err, "admiral run", id)
			}
			return w.finish(cmd, opts, c, resp.GetRun())
		},
	}
	w.register(cmd, true, "the apply ends")
	return cmd
}

func newAcceptCmd(opts *client.Options) *cobra.Command {
	var (
		reason string
		yes    bool
	)
	cmd := &cobra.Command{
		Use:   "accept <run>",
		Short: "Accept a partial apply as the environment's state",
		Long: `Accept a run that applied partway. The environment is released, with the
components that applied recorded as live and the rest as they were. The
command asks first; --yes skips the question.`,
		Example: `  admiral run accept run-7f2a1c9d0e3b --reason "fixing forward"`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := runID(args[0])
			if err != nil {
				return err
			}
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			if err := input.Confirm(cmd, yes, "Accept what "+id+" applied as the environment's state"); err != nil {
				return err
			}
			resp, err := c.Run().AcceptRun(cmd.Context(), &runv1.AcceptRunRequest{RunId: id, Reason: reason})
			if err != nil {
				return err
			}
			return printTitled(cmd, output.NewPrinter(cmd, opts.OutputFormat), c, resp.GetRun())
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded with the acceptance")
	_ = cmd.MarkFlagRequired("reason")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
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
			r, err := getRun(cmd.Context(), c.Run(), id)
			if err != nil {
				return err
			}
			WriteDiff(p.Out(), resp.GetDiff(), r.GetPlan())
			return nil
		},
	}
}

// WriteDiff writes a plan's diffs resource by resource, as 'run diff'
// and the changeset verbs that show a plan before acting print them. A
// delete is one line: the whole object, minus, says nothing more. plan
// supplies each resource's action; without it every diff is printed whole.
func WriteDiff(w io.Writer, d *agentv1.PlanDiff, plan *runv1.PlanPhase) {
	actions := map[string]agentv1.ResourceChange_Action{}
	removed := map[string]bool{}
	for _, c := range plan.GetComponents() {
		all := len(c.GetChanges()) > 0
		for _, rc := range c.GetChanges() {
			actions[resourceKey(c.GetComponent(), rc.GetResource())] = rc.GetAction()
			all = all && rc.GetAction() == agentv1.ResourceChange_ACTION_DESTROY
		}
		removed[c.GetComponent()] = all
	}
	kept := map[string]bool{}
	var namespaces []string
	for _, rd := range d.GetResources() {
		r := rd.GetResource()
		if actions[resourceKey(rd.GetComponent(), r)] == agentv1.ResourceChange_ACTION_DESTROY {
			output.Writef(w, "- %s: %s %s\n", rd.GetComponent(), r.GetApiVersion(), deletedName(r))
			if ns := r.GetNamespace(); ns != "" && removed[rd.GetComponent()] && !kept[ns] {
				kept[ns] = true
				namespaces = append(namespaces, ns)
			}
			continue
		}
		output.Writef(w, "# %s: %s\n%s\n", rd.GetComponent(), resourceName(r), rd.GetDiff())
	}
	for _, ns := range namespaces {
		output.Writef(w, "The namespace %s stays; Admiral never deletes a namespace.\n", ns)
	}
}

func resourceKey(component string, r *agentv1.ResourceRef) string {
	return component + "\x00" + r.GetApiVersion() + "\x00" + resourceName(r)
}

func deletedName(r *agentv1.ResourceRef) string {
	if r.GetNamespace() == "" {
		return r.GetKind() + " " + r.GetName()
	}
	return r.GetKind() + " " + r.GetNamespace() + "/" + r.GetName()
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
