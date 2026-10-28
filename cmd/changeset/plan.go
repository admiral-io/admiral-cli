package changeset

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	runcmd "go.admiral.io/cli/cmd/run"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// defaultWaitTimeout caps a wait for a plan.
const defaultWaitTimeout = 5 * time.Minute

func newPlanCmd(opts *client.Options) *cobra.Command {
	var (
		ifRev int32
		po    = planOptions{plan: true}
	)

	cmd := &cobra.Command{
		Use:               "plan <changeset>",
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		Short:             "Plan a changeset's head revision",
		Long: `Plan a changeset's head revision.

This starts a run. The run prepares the revision first, rendering every
component it touches into one artifact that 'admiral changeset get
--rendered' reads, then an agent of the environment plans it.

The command prints the run's ID, waits until the plan is done and prints
the run and its diff, resource by resource. It exits 1 if the run fails or
is superseded, 124 if --wait-timeout elapses; the run continues on the
server either way. Approve and apply the plan with 'admiral changeset
approve' and 'admiral changeset apply'.`,
		Example: `  admiral changeset plan cs-7f2a1c9d0e3b

  # Start the run and return
  admiral changeset plan cs-7f2a1c9d0e3b --no-wait

  # Refuse if someone else edited it since revision 4
  admiral changeset plan cs-7f2a1c9d0e3b --if-revision 4`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			if err := po.check(cmd); err != nil {
				return err
			}

			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
			if err != nil {
				return err
			}
			if resp.Head == nil {
				return cmderr.WithHint(fmt.Errorf("%s has no revision to plan", csID),
					"To edit it, run: admiral changeset set "+csID+" <component>.<path>=<value>")
			}
			if want := ifRevision(cmd, ifRev); want != nil && *want != resp.Head.Number {
				return cmderr.WithHint(fmt.Errorf("%s is at revision %d, not %d", csID, resp.Head.Number, *want),
					"To see the head, run: admiral changeset diff "+csID)
			}
			return planRevision(cmd, opts, c.Run(), resp.ChangeSet, resp.Head, po)
		},
	}

	revisionFlag(cmd, &ifRev)
	waitFlags(cmd, &po)

	return cmd
}

// planOptions is what a command that asks for a plan was told about
// waiting for it. The edit verbs ask only with --plan; plan always asks.
type planOptions struct {
	plan        bool
	noWait      bool
	waitTimeout time.Duration
	// full shows the plan's resources whole rather than in brief.
	full bool
}

func waitFlags(cmd *cobra.Command, po *planOptions) {
	waitFlagsFor(cmd, po, "the plan")
}

// waitFlagsFor names what --wait-timeout waits for: the plan, or on apply
// the apply.
func waitFlagsFor(cmd *cobra.Command, po *planOptions, what string) {
	cmd.Flags().BoolVar(&po.noWait, "no-wait", false, "return once the run is started")
	cmd.Flags().DurationVar(&po.waitTimeout, "wait-timeout", defaultWaitTimeout,
		"how long to wait for "+what+"; exits 124 when it elapses")
	cmd.Flags().BoolVar(&po.full, "full", false, "show each changed resource whole, as YAML, with its changed lines marked")
}

// planFlags registers --plan and its wait flags on an edit verb.
func planFlags(cmd *cobra.Command, po *planOptions) {
	cmd.Flags().BoolVar(&po.plan, "plan", false, "plan the revision this cuts, and wait for the plan")
	waitFlags(cmd, po)
}

func (po *planOptions) check(cmd *cobra.Command) error {
	if !po.plan && (cmd.Flags().Changed("no-wait") || cmd.Flags().Changed("wait-timeout") || po.full) {
		return cmderr.Usage("--no-wait, --wait-timeout and --full need --plan")
	}
	if po.waitTimeout <= 0 {
		return cmderr.Usage("--wait-timeout must be positive")
	}
	return nil
}

// planRevision starts the run that plans rev and, unless --no-wait, waits
// for it to settle. The exit code is the run's outcome.
func planRevision(cmd *cobra.Command, opts *client.Options, runs runv1.RunAPIClient, cs *changesetv1.ChangeSet, rev *changesetv1.Revision, po planOptions) error {
	if rev == nil {
		return fmt.Errorf("the server returned no revision for %s", cs.GetId())
	}
	resp, err := runs.CreateRun(cmd.Context(), &runv1.CreateRunRequest{
		EnvironmentId: cs.EnvironmentId,
		Run: &runv1.Run{
			ChangeSetId: cs.Id,
			Source:      &runv1.Run_RevisionId{RevisionId: rev.Id},
		},
	})
	if err != nil {
		return err
	}
	r := resp.Run
	p := output.NewPrinter(cmd, opts.OutputFormat)
	progress := runcmd.NewProgress(p.Err(), false)
	output.Writef(progress, "Planning %s revision %d in %s\n", cs.Id, rev.Number, r.Id)

	if !po.noWait {
		done, err := runcmd.Wait(cmd.Context(), runs, r, po.waitTimeout, true, progress)
		if err != nil {
			return runcmd.WaitError(err, r)
		}
		r = done
	}
	progress.End()
	// Under -o json|yaml the run is the whole answer; 'run diff' has the rest.
	withDiff := r.GetStatus() == runv1.Run_STATUS_PLANNED && opts.OutputFormat.IsTable()
	if !withDiff {
		if err := runcmd.Print(p, r, cs.GetTitle()); err != nil {
			return err
		}
		return runcmd.Outcome(r)
	}
	diff, err := runs.GetRunPlanDiff(cmd.Context(), &runv1.GetRunPlanDiffRequest{RunId: r.GetId()})
	if err != nil {
		return err
	}
	if err := runcmd.PrintWithDiff(p, r, cs.GetTitle(), func(w io.Writer) {
		writePlan(w, diff.GetDiff(), r, po.full)
	}); err != nil {
		return err
	}
	return runcmd.Outcome(r)
}

func shortArtifact(d string) string {
	if d == "" {
		return ""
	}
	return "sha256:" + shortDigest(d)
}
