package changeset

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// defaultWaitTimeout caps a wait for a prepare.
const defaultWaitTimeout = 5 * time.Minute

// The poll starts at pollFirst and grows by half each time up to pollMax,
// so a quick render answers quickly and a slow one is not hammered.
var (
	pollFirst = time.Second
	pollMax   = 10 * time.Second
)

func newPlanCmd(opts *client.Options) *cobra.Command {
	var (
		ifRev int32
		po    = planOptions{plan: true}
	)

	cmd := &cobra.Command{
		Use:   "plan <change-set>",
		Short: "Plan a change set's head revision",
		Long: `Plan a change set's head revision.

The revision is prepared first: every component it touches is rendered into
one run artifact, which 'admiral changeset get --rendered' reads. Planning
against the cluster comes later; for now a plan ends when the revision is
prepared.

The command waits for the prepare and prints its status, the artifact digest
and any findings. It polls with a growing interval; exits 1 if the prepare
fails or is superseded, 124 if --wait-timeout elapses. Asking again for the
same revision returns the same prepare, so running it again resumes a wait.`,
		Example: `  admiral changeset plan cs-7f2a1c9d0e3b

  # Queue it and return
  admiral changeset plan cs-7f2a1c9d0e3b --no-wait

  # Refuse if someone else edited it since revision 4
  admiral changeset plan cs-7f2a1c9d0e3b --if-revision 4`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			if err := po.check(cmd); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().PlanChangeSet(cmd.Context(), &changesetv1.PlanChangeSetRequest{
				ChangeSetId: csID,
				IfRevision:  ifRevision(cmd, ifRev),
			})
			if err != nil {
				return err
			}
			return finishPrepare(cmd, opts, c.ChangeSet(), csID, resp.Prepare, po)
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
}

func waitFlags(cmd *cobra.Command, po *planOptions) {
	cmd.Flags().BoolVar(&po.noWait, "no-wait", false, "return once the prepare is queued")
	cmd.Flags().DurationVar(&po.waitTimeout, "wait-timeout", defaultWaitTimeout,
		"how long to wait for the prepare; exits 124 when it elapses")
}

// planFlags registers --plan and its wait flags on an edit verb.
func planFlags(cmd *cobra.Command, po *planOptions) {
	cmd.Flags().BoolVar(&po.plan, "plan", false, "plan the revision this cuts, and wait for its prepare")
	waitFlags(cmd, po)
}

func (po *planOptions) check(cmd *cobra.Command) error {
	if !po.plan && (cmd.Flags().Changed("no-wait") || cmd.Flags().Changed("wait-timeout")) {
		return cmderr.Usage("--no-wait and --wait-timeout need --plan")
	}
	if po.waitTimeout <= 0 {
		return cmderr.Usage("--wait-timeout must be positive")
	}
	return nil
}

// finishPrepare prints a prepare the server just returned, or waits for it
// to finish and prints that. The exit code is the prepare's outcome.
func finishPrepare(cmd *cobra.Command, opts *client.Options, c changesetv1.ChangeSetAPIClient, csID string, prep *changesetv1.Prepare, po planOptions) error {
	if prep == nil {
		return fmt.Errorf("the server returned no prepare for %s", csID)
	}
	p := output.NewPrinter(cmd, opts.OutputFormat)
	if !po.noWait && !finished(prep.Status) {
		output.Writef(p.Err(), "Preparing %s revision %d (%s)\n", csID, prep.Revision, output.FormatEnumKebab(prep.Status))
		rev := prep.Revision
		var err error
		prep, err = waitPrepare(cmd.Context(), prep, po.waitTimeout,
			func(ctx context.Context) (*changesetv1.Prepare, error) {
				resp, err := c.GetPrepare(ctx, &changesetv1.GetPrepareRequest{ChangeSetId: csID, Revision: rev})
				return resp.GetPrepare(), err
			},
			func(next *changesetv1.Prepare) {
				if !finished(next.Status) {
					output.Writef(p.Err(), "Revision %d is %s\n", next.Revision, output.FormatEnumKebab(next.Status))
				}
			})
		if err != nil {
			return prepareTimeout(err, csID, rev)
		}
	}
	if err := printPrepare(p, prep); err != nil {
		return err
	}
	return prepareOutcome(csID, prep)
}

func finished(s changesetv1.PrepareStatus) bool {
	switch s {
	case changesetv1.PrepareStatus_PREPARED, changesetv1.PrepareStatus_FAILED, changesetv1.PrepareStatus_SUPERSEDED:
		return true
	}
	return false
}

// errWaitElapsed is waitPrepare's timeout, which the caller names.
type errWaitElapsed struct{ last *changesetv1.Prepare }

func (e errWaitElapsed) Error() string { return "wait elapsed" }

// waitPrepare polls until the prepare finishes or timeout elapses, calling
// changed whenever the status moves. A canceled context ends the wait, not
// the prepare.
func waitPrepare(ctx context.Context, cur *changesetv1.Prepare, timeout time.Duration,
	get func(context.Context) (*changesetv1.Prepare, error), changed func(*changesetv1.Prepare),
) (*changesetv1.Prepare, error) {
	deadline := time.Now().Add(timeout)
	delay := pollFirst
	for !finished(cur.Status) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return cur, errWaitElapsed{last: cur}
		}
		t := time.NewTimer(min(delay, remaining))
		select {
		case <-ctx.Done():
			t.Stop()
			return cur, ctx.Err()
		case <-t.C:
		}
		next, err := get(ctx)
		if err != nil {
			return cur, err
		}
		if next.Status != cur.Status {
			changed(next)
		}
		cur = next
		delay = min(delay*3/2, pollMax)
	}
	return cur, nil
}

func prepareTimeout(err error, csID string, rev int32) error {
	e, ok := err.(errWaitElapsed)
	if !ok {
		return err
	}
	return &cmderr.Error{
		Err: fmt.Errorf("%s revision %d is still %s; it continues on the server",
			csID, rev, output.FormatEnumKebab(e.last.Status)),
		Code: cmderr.ExitTimeout,
		Hint: fmt.Sprintf("Run 'admiral changeset plan %s --if-revision %d' to wait again.", csID, rev),
	}
}

// prepareOutcome is the exit status of a finished prepare. A prepare still
// queued or running (--no-wait) is not a failure.
func prepareOutcome(csID string, prep *changesetv1.Prepare) error {
	switch prep.Status {
	case changesetv1.PrepareStatus_FAILED:
		e := prep.GetError()
		msg := e.GetMessage()
		if e.GetComponent() != "" {
			msg = e.GetComponent() + ": " + msg
		}
		hint := fmt.Sprintf("Fix the inputs and edit the change set; planning revision %d again gives the same result.", prep.Revision)
		if e.GetClass() == changesetv1.PrepareErrorClass_INFRA {
			hint = fmt.Sprintf("Run 'admiral changeset plan %s --if-revision %d' to queue it again.", csID, prep.Revision)
		}
		return cmderr.WithHint(fmt.Errorf("%s revision %d failed to prepare (%s error): %s",
			csID, prep.Revision, output.FormatEnumKebab(e.GetClass()), msg), hint)
	case changesetv1.PrepareStatus_SUPERSEDED:
		return cmderr.WithHint(fmt.Errorf("%s revision %d was superseded by a newer revision before it was prepared",
			csID, prep.Revision), fmt.Sprintf("Run 'admiral changeset plan %s' to plan the head.", csID))
	}
	return nil
}

// prepareTable is the one row a prepare prints as.
var prepareTable = output.Table[*changesetv1.Prepare]{
	{Header: "REVISION", Cell: func(p *changesetv1.Prepare) string { return strconv.Itoa(int(p.Revision)) }},
	{Header: "STATUS", Cell: func(p *changesetv1.Prepare) string { return output.FormatEnumKebab(p.Status) }},
	{Header: "ARTIFACT", Cell: func(p *changesetv1.Prepare) string { return shortArtifact(p.ArtifactDigest) }},
	{Header: "FINDINGS", Cell: func(p *changesetv1.Prepare) string { return strconv.Itoa(len(p.Findings)) }},
	{Header: "ARTIFACT-DIGEST", Wide: true, Cell: func(p *changesetv1.Prepare) string { return p.ArtifactDigest }},
	{Header: "REQUESTED-BY", Wide: true, Cell: func(p *changesetv1.Prepare) string { return output.FormatActor(p.RequestedBy) }},
	{Header: "ATTEMPTS", Wide: true, Cell: func(p *changesetv1.Prepare) string { return strconv.Itoa(int(p.Attempts)) }},
}

func shortArtifact(d string) string {
	if d == "" {
		return ""
	}
	return "sha256:" + shortDigest(d)
}

// printPrepare prints the prepare row, then its findings. A failure's
// message is the command's error line, so it is not repeated here.
func printPrepare(p *output.Printer, prep *changesetv1.Prepare) error {
	return p.PrintResource(prep, func(w *tabwriter.Writer) {
		prepareTable.Write(w, p, prep)
		if len(prep.Findings) == 0 {
			return
		}
		output.Writeln(w)
		writeFindings(w, prep.Findings)
	})
}

func writeFindings(w *tabwriter.Writer, fs []*changesetv1.Finding) {
	output.Writeln(w, "COMPONENT\tCODE\tMESSAGE\tDETAILS")
	for _, f := range fs {
		details := strings.Join(f.Details, "; ")
		if details == "" {
			details = output.None
		}
		output.Writef(w, "%s\t%s\t%s\t%s\n", f.Component, output.FormatEnumKebab(f.Code), f.Message, details)
	}
}
