package run

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

type stubRuns struct {
	runv1.RunAPIClient
	next  *runv1.Run
	polls int
}

func (s *stubRuns) GetRun(context.Context, *runv1.GetRunRequest, ...grpc.CallOption) (*runv1.GetRunResponse, error) {
	s.polls++
	return &runv1.GetRunResponse{Run: s.next}, nil
}

func queuedOnUnenrolled() *runv1.Run {
	return &runv1.Run{
		Id: testRunID, Status: runv1.Run_STATUS_QUEUED, WaitReason: agentv1.WaitReason_WAIT_REASON_AGENT_NOT_ENROLLED,
		Plan: &runv1.PlanPhase{Jobs: []*runv1.RunJob{{
			JobId: "job-1", AgentId: "agt-prod", WaitReason: agentv1.WaitReason_WAIT_REASON_AGENT_NOT_ENROLLED,
		}}},
	}
}

// An agent that has never enrolled will not come up by itself, so the wait
// ends at once, exits 1 and leaves the run queued.
func TestWaitStopsOnAnUnenrolledAgent(t *testing.T) {
	noPollDelay(t)

	stub := &stubRuns{next: queuedOnUnenrolled()}
	start := &runv1.Run{Id: testRunID, Status: runv1.Run_STATUS_PREPARING}
	last, err := Wait(context.Background(), stub, start, DefaultWaitTimeout, true, io.Discard)
	require.Error(t, err)
	assert.Equal(t, 1, stub.polls, "stops on the first poll that shows the agent unenrolled")
	assert.Equal(t, runv1.Run_STATUS_QUEUED, last.GetStatus())

	err = WaitError(err, start)
	assert.Equal(t, "run "+testRunID+" is queued: agent agt-prod has never enrolled", err.Error())
	assert.Equal(t, cmderr.ExitError, cmderr.Code(err))
	assert.Contains(t, cmderr.Hint(err), "admiral agent enrollment-key agt-prod")
	assert.Contains(t, cmderr.Hint(err), "admiral run get "+testRunID+" --wait")
}

// Following a run waits through its agent's install.
func TestWaitFollowsThroughEnrollment(t *testing.T) {
	noPollDelay(t)

	stub := &stubRuns{next: queuedOnUnenrolled()}
	start := &runv1.Run{Id: testRunID, Status: runv1.Run_STATUS_PREPARING}
	_, err := Wait(context.Background(), stub, start, 50*time.Millisecond, false, io.Discard)
	assert.IsType(t, errWaitElapsed{}, err, "only the timeout ends it")
	assert.Greater(t, stub.polls, 1)
}

// Any other reason keeps waiting.
func TestWaitKeepsWaitingOnAnOfflineAgent(t *testing.T) {
	noPollDelay(t)

	offline := &runv1.Run{Id: testRunID, Status: runv1.Run_STATUS_QUEUED, WaitReason: agentv1.WaitReason_WAIT_REASON_AGENT_OFFLINE}
	stub := &stubRuns{next: &runv1.Run{Id: testRunID, Status: runv1.Run_STATUS_NO_CHANGES}}
	last, err := Wait(context.Background(), stub, offline, DefaultWaitTimeout, true, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, runv1.Run_STATUS_NO_CHANGES, last.GetStatus())
}

func noPollDelay(t *testing.T) {
	old := pollFirst
	pollFirst = 0
	t.Cleanup(func() { pollFirst = old })
}

// What a command prints while the user waits is one group, and the record
// after it the next, so a blank line comes between them; with nothing
// printed there is nothing to set apart.
func TestProgressSetsTheRecordApart(t *testing.T) {
	var b bytes.Buffer
	p := NewProgress(&b, false)
	p.End()
	assert.Empty(t, b.String())

	output.Writef(p, "Run run-1 is planning\n")
	p.End()
	assert.Equal(t, "Run run-1 is planning\n\n", b.String())

	b.Reset()
	NewProgress(&b, true).End()
	assert.Equal(t, "\n", b.String(), "a prompt the user answered began the group")
}
