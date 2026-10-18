package run

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// DefaultWaitTimeout caps a wait for a run to settle.
const DefaultWaitTimeout = 15 * time.Minute

// The poll starts at pollFirst and grows by half up to pollMax.
var (
	pollFirst = time.Second
	pollMax   = 10 * time.Second
)

// Settled reports whether a run waits on people or has ended: nothing more
// happens to it without someone acting.
func Settled(s runv1.Run_Status) bool {
	switch s {
	case runv1.Run_STATUS_PREPARING, runv1.Run_STATUS_QUEUED, runv1.Run_STATUS_PLANNING,
		runv1.Run_STATUS_APPLYING, runv1.Run_STATUS_UNSPECIFIED:
		return false
	}
	return true
}

// settled is Settled, and for a planned run also that its plan is readable:
// the status can turn planned a moment before the plan it names is recorded.
func settled(r *runv1.Run) bool {
	if r.GetStatus() == runv1.Run_STATUS_PLANNED && r.GetPlan().GetPlanDigest() == "" {
		return false
	}
	return Settled(r.GetStatus())
}

type errWaitElapsed struct{ last *runv1.Run }

func (errWaitElapsed) Error() string { return "wait elapsed" }

// Wait polls until the run settles or timeout elapses, writing a line to
// progress whenever its status moves. A canceled context ends the wait, not
// the run.
func Wait(ctx context.Context, c runv1.RunAPIClient, cur *runv1.Run, timeout time.Duration, progress io.Writer) (*runv1.Run, error) {
	if !Settled(cur.GetStatus()) {
		output.Writef(progress, "Run %s is %s\n", cur.GetId(), progressStatus(cur))
	}
	deadline := time.Now().Add(timeout)
	delay := pollFirst
	for !settled(cur) {
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
		resp, err := c.GetRun(ctx, &runv1.GetRunRequest{RunId: cur.GetId()})
		if err != nil {
			return cur, err
		}
		next := resp.GetRun()
		if progressStatus(next) != progressStatus(cur) && !Settled(next.GetStatus()) {
			output.Writef(progress, "Run %s is %s\n", next.GetId(), progressStatus(next))
		}
		cur = next
		delay = min(delay*3/2, pollMax)
	}
	return cur, nil
}

// WaitTimeout turns an elapsed wait into exit 124 naming where the run is;
// any other error passes through.
func WaitTimeout(err error, r *runv1.Run) error {
	e, ok := err.(errWaitElapsed)
	if !ok {
		return err
	}
	return &cmderr.Error{
		Err:  fmt.Errorf("run %s is still %s; it continues on the server", e.last.GetId(), describeStatus(e.last)),
		Code: cmderr.ExitTimeout,
		Hint: fmt.Sprintf("Run 'admiral run get %s --wait' to wait again.", r.GetId()),
	}
}

// Outcome is the exit status of a settled run. A run still moving
// (--no-wait) is not a failure.
func Outcome(r *runv1.Run) error {
	switch r.GetStatus() {
	case runv1.Run_STATUS_FAILED:
		msg := r.GetApply().GetMessage()
		if r.GetApply() == nil {
			msg = r.GetPlan().GetMessage()
		}
		return cmderr.Reported(fmt.Errorf("run %s failed: %s", r.GetId(), msg))
	case runv1.Run_STATUS_PARTIAL:
		return cmderr.Reported(&cmderr.Error{
			Err:  fmt.Errorf("run %s applied partway; the environment is held", r.GetId()),
			Code: cmderr.ExitError,
			Hint: HoldHint(r.GetChangeSetId(), r.GetId()),
		})
	case runv1.Run_STATUS_SUPERSEDED:
		return cmderr.Reported(fmt.Errorf("run %s was superseded by a newer plan", r.GetId()))
	}
	return nil
}

var changeSetIDPattern = regexp.MustCompile(`cs-[0-9a-z]{12}`)

// HoldHint is how to release an environment a partial apply holds: undo the
// changeset, or accept what landed.
func HoldHint(changeSetID, runID string) string {
	accept := "'admiral run accept " + runID + " --reason ...'"
	if runID == "" {
		accept = "'admiral run accept <run> --reason ...'"
	}
	if changeSetID == "" {
		return "Run " + accept + " to accept what landed."
	}
	return fmt.Sprintf("Run 'admiral changeset revert %s' to undo it, or %s to accept what landed.", changeSetID, accept)
}

// waitWords says why a queued run waits, for the progress line.
var waitWords = map[agentv1.WaitReason]string{
	agentv1.WaitReason_WAIT_REASON_NO_AGENT_ASSIGNED:     "waiting: the environment has no agent",
	agentv1.WaitReason_WAIT_REASON_AGENT_NOT_GRANTED:     "waiting: the agent is not granted to this environment",
	agentv1.WaitReason_WAIT_REASON_AGENT_OFFLINE:         "waiting: the agent is offline",
	agentv1.WaitReason_WAIT_REASON_AGENT_AT_CAPACITY:     "waiting for the agent to finish other work",
	agentv1.WaitReason_WAIT_REASON_AGENT_TOO_OLD:         "waiting: the agent is too old for this job",
	agentv1.WaitReason_WAIT_REASON_ENVIRONMENT_BUSY:      "waiting: another apply holds the environment",
	agentv1.WaitReason_WAIT_REASON_AGENT_NOT_ENROLLED:    "waiting: the agent has not enrolled",
	agentv1.WaitReason_WAIT_REASON_PROFILE_MISSING:       "waiting: the agent has not reported the profile this job needs",
	agentv1.WaitReason_WAIT_REASON_COMPONENT_NOT_ALLOWED: "waiting: the agent no longer allows a component of this job",
}

// progressStatus is the run's status as the progress line says it. A
// reason that needs no one to act reads as waiting for an agent.
func progressStatus(r *runv1.Run) string {
	if r.GetStatus() != runv1.Run_STATUS_QUEUED {
		return output.FormatEnumKebab(r.GetStatus())
	}
	if w, ok := waitWords[r.GetWaitReason()]; ok {
		return w
	}
	return "waiting for an agent"
}

func describeStatus(r *runv1.Run) string {
	s := output.FormatEnumKebab(r.GetStatus())
	if r.GetStatus() == runv1.Run_STATUS_QUEUED && r.GetWaitReason() != 0 {
		s += " (" + output.FormatEnumKebab(r.GetWaitReason()) + ")"
	}
	return s
}

// Refusal gives an approve or apply the platform refused a hint that says
// what would unblock it: approvals, or a partial apply holding the
// environment. verb and target address the plan, as "admiral changeset" and
// a changeset, or "admiral run" and a run.
func Refusal(err error, verb, target string) error {
	msg := status.Convert(err).Message()
	switch {
	case status.Code(err) == codes.FailedPrecondition && strings.Contains(msg, "holds the environment"):
		return cmderr.WithHint(err, HoldHint(changeSetIDPattern.FindString(msg), ""))
	case status.Code(err) == codes.PermissionDenied && strings.Contains(msg, "author"):
		return cmderr.WithHint(err, "The changeset's author never counts toward its approvals; someone else must run '"+
			verb+" approve "+target+"'.")
	case status.Code(err) == codes.FailedPrecondition && strings.Contains(msg, "approval"):
		return cmderr.WithHint(err, "Someone other than the changeset's author must run '"+verb+" approve "+target+"' first.")
	}
	return err
}
