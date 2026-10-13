package run

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
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

type errWaitElapsed struct{ last *runv1.Run }

func (errWaitElapsed) Error() string { return "wait elapsed" }

// Wait polls until the run settles or timeout elapses, writing a line to
// progress whenever its status moves. A canceled context ends the wait, not
// the run.
func Wait(ctx context.Context, c runv1.RunAPIClient, cur *runv1.Run, timeout time.Duration, progress io.Writer) (*runv1.Run, error) {
	if !Settled(cur.GetStatus()) {
		output.Writef(progress, "Run %s is %s\n", cur.GetId(), describeStatus(cur))
	}
	deadline := time.Now().Add(timeout)
	delay := pollFirst
	for !Settled(cur.GetStatus()) {
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
		if next.GetStatus() != cur.GetStatus() && !Settled(next.GetStatus()) {
			output.Writef(progress, "Run %s is %s\n", next.GetId(), describeStatus(next))
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
			Hint: fmt.Sprintf("Revert the changeset, or run 'admiral run accept %s --reason ...'.", r.GetId()),
		})
	case runv1.Run_STATUS_SUPERSEDED:
		return cmderr.Reported(fmt.Errorf("run %s was superseded by a newer plan", r.GetId()))
	}
	return nil
}

func describeStatus(r *runv1.Run) string {
	s := output.FormatEnumKebab(r.GetStatus())
	if r.GetStatus() == runv1.Run_STATUS_QUEUED && r.GetWaitReason() != 0 {
		s += " (" + output.FormatEnumKebab(r.GetWaitReason()) + ")"
	}
	return s
}
