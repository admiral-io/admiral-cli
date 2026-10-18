package run

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

const (
	csID      = "cs-7f2a1c9d0e3b"
	testRunID = "run-1nnsyzr305q7"
)

func render(t *testing.T, printer func(*output.Printer, *runv1.Run) error, r *runv1.Run) string {
	t.Helper()
	cmd := &cobra.Command{}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, printer(output.NewPrinter(cmd, output.FormatTable), r))
	return out.String() + errOut.String()
}

func change(kind, name string, a agentv1.ResourceChange_Action) *agentv1.ResourceChange {
	return &agentv1.ResourceChange{Action: a, Resource: &agentv1.ResourceRef{ApiVersion: "v1", Kind: kind, Namespace: "podinfo-dev", Name: name}}
}

func planned(rule *environmentv1.ApprovalRule) *runv1.Run {
	return &runv1.Run{
		Id: testRunID, ChangeSetId: csID, Source: &runv1.Run_RevisionId{RevisionId: "rev-q8tx5359s1nk"},
		Status: runv1.Run_STATUS_PLANNED, RequiredApproval: rule,
		Plan: &runv1.PlanPhase{PlanDigest: "sha256:4965", Components: []*agentv1.ComponentPlan{{
			Component: "podinfo", Changes: []*agentv1.ResourceChange{change("Deployment", "podinfo", agentv1.ResourceChange_ACTION_CREATE)},
		}}},
	}
}

// A planned run points at the changeset, and at approving only when the
// environment asks for approvals.
func TestPlannedRunHints(t *testing.T) {
	out := render(t, Print, planned(nil))
	assert.Contains(t, out, "To apply it, run: admiral changeset apply "+csID)
	assert.Contains(t, out, "admiral run diff "+testRunID)
	assert.NotContains(t, out, "approve")
	assert.NotContains(t, out, "admiral run apply")

	out = render(t, PrintBeforeDiff, planned(&environmentv1.ApprovalRule{RequiredApprovals: 1}))
	assert.NotContains(t, out, "run diff", "the diff follows")
	assert.Contains(t, out, "It needs 1 approval(s) from someone other than the changeset's author: admiral changeset approve "+csID)

	groups := planned(&environmentv1.ApprovalRule{Groups: []*environmentv1.ApprovalRule_GroupApproval{{GroupId: "grp-1", Approvals: 2}}})
	assert.Contains(t, render(t, Print, groups), "It needs 2 approval(s)")

	desired := planned(nil)
	desired.ChangeSetId, desired.Source = "", &runv1.Run_Desired{Desired: &runv1.Desired{Mode: runv1.Desired_MODE_REAPPLY}}
	assert.Contains(t, render(t, Print, desired), "To apply it, run: admiral run apply "+testRunID, "a run without a changeset is acted on directly")
}

func TestApplyShowsWhatEachComponentChanged(t *testing.T) {
	r := planned(nil)
	r.Status = runv1.Run_STATUS_APPLIED
	r.Apply = &runv1.ApplyPhase{Components: []*runv1.ComponentApply{
		{Component: "podinfo", Status: runv1.ComponentApply_STATUS_SUCCEEDED},
	}}
	out := render(t, Print, r)
	assert.Regexp(t, `podinfo\s+succeeded\s+\+1 ~0 ±0 -0`, out)
}

func TestWriteDiffPrintsADeleteAsOneLine(t *testing.T) {
	plan := &runv1.PlanPhase{Components: []*agentv1.ComponentPlan{
		{Component: "podinfo", Changes: []*agentv1.ResourceChange{
			change("Deployment", "podinfo", agentv1.ResourceChange_ACTION_DESTROY),
			change("Service", "podinfo", agentv1.ResourceChange_ACTION_DESTROY),
		}},
		{Component: "web", Changes: []*agentv1.ResourceChange{
			change("ConfigMap", "web", agentv1.ResourceChange_ACTION_UPDATE),
			change("Secret", "old", agentv1.ResourceChange_ACTION_DESTROY),
		}},
	}}
	ref := func(kind, name string) *agentv1.ResourceRef {
		return &agentv1.ResourceRef{ApiVersion: "v1", Kind: kind, Namespace: "podinfo-dev", Name: name}
	}
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{
		{Component: "podinfo", Resource: ref("Deployment", "podinfo"), Diff: "--- live\n+++ desired\n-apiVersion: apps/v1\n-kind: Deployment\n"},
		{Component: "podinfo", Resource: ref("Service", "podinfo"), Diff: "--- live\n-kind: Service\n"},
		{Component: "web", Resource: ref("ConfigMap", "web"), Diff: "-a: 1\n+a: 2\n"},
		{Component: "web", Resource: ref("Secret", "old"), Diff: "-kind: Secret\n"},
	}}
	var b bytes.Buffer
	WriteDiff(&b, d, plan)
	out := b.String()
	assert.Contains(t, out, "- podinfo: v1 Deployment podinfo-dev/podinfo\n")
	assert.Contains(t, out, "- podinfo: v1 Service podinfo-dev/podinfo\n")
	assert.NotContains(t, out, "kind: Deployment", "a delete is not printed whole")
	assert.Contains(t, out, "# web: podinfo-dev/ConfigMap/web\n-a: 1\n+a: 2")
	assert.Equal(t, 1, strings.Count(out, "The namespace podinfo-dev stays"),
		"said once, and only for a component removed outright")

	b.Reset()
	WriteDiff(&b, d, nil)
	assert.Contains(t, b.String(), "kind: Deployment", "without the plan every diff is printed whole")
}

func TestProgressStatusSaysWhyARunWaits(t *testing.T) {
	q := func(w agentv1.WaitReason) *runv1.Run {
		return &runv1.Run{Status: runv1.Run_STATUS_QUEUED, WaitReason: w}
	}
	assert.Equal(t, "waiting for an agent", progressStatus(q(agentv1.WaitReason_WAIT_REASON_BACKOFF)))
	assert.Equal(t, "waiting for an agent", progressStatus(q(agentv1.WaitReason_WAIT_REASON_UNSPECIFIED)))
	assert.Equal(t, "waiting: the agent is offline", progressStatus(q(agentv1.WaitReason_WAIT_REASON_AGENT_OFFLINE)))
	assert.Equal(t, "planning", progressStatus(&runv1.Run{Status: runv1.Run_STATUS_PLANNING}))
}

// The status can say planned a moment before the plan is recorded; the wait
// goes on until the plan is readable, or the summary prints empty.
func TestAPlannedRunIsSettledOnceItsPlanIsRecorded(t *testing.T) {
	assert.False(t, settled(&runv1.Run{Status: runv1.Run_STATUS_PLANNED}))
	assert.True(t, settled(&runv1.Run{Status: runv1.Run_STATUS_PLANNED, Plan: &runv1.PlanPhase{PlanDigest: "sha256:1"}}))
	assert.True(t, settled(&runv1.Run{Status: runv1.Run_STATUS_NO_CHANGES}))
	assert.False(t, settled(&runv1.Run{Status: runv1.Run_STATUS_APPLYING}))
}

func TestRefusalSaysWhatUnblocksIt(t *testing.T) {
	author := status.Error(codes.PermissionDenied, "you revised change set "+csID+"; the author never counts toward an approval")
	err := Refusal(author, "admiral changeset", csID)
	assert.Equal(t, "The changeset's author never counts toward its approvals; someone else must run 'admiral changeset approve "+csID+"'.", cmderr.Hint(err))

	needs := status.Error(codes.FailedPrecondition, "the plan needs 1 approval(s) and has 0")
	assert.Contains(t, cmderr.Hint(Refusal(needs, "admiral run", testRunID)), "must run 'admiral run approve "+testRunID+"' first")

	held := status.Error(codes.FailedPrecondition, "change set "+csID+"'s partial apply holds the environment; revert it or accept it first")
	assert.Equal(t, "Run 'admiral changeset revert "+csID+"' to undo it, or 'admiral run accept <run> --reason ...' to accept what landed.",
		cmderr.Hint(Refusal(held, "admiral changeset", "cs-other0000000")))

	scope := status.Error(codes.PermissionDenied, "missing scope run:write")
	assert.Empty(t, cmderr.Hint(Refusal(scope, "admiral run", testRunID)), "a real scope refusal keeps the root's scope hint")
}

func TestPartialOutcomeNamesRevertAndAccept(t *testing.T) {
	err := Outcome(&runv1.Run{Id: testRunID, ChangeSetId: csID, Status: runv1.Run_STATUS_PARTIAL})
	assert.Equal(t, "Run 'admiral changeset revert "+csID+"' to undo it, or 'admiral run accept "+testRunID+" --reason ...' to accept what landed.",
		cmderr.Hint(err))
}

func execRun(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv(credentials.EnvAPIKey, "")
	root := NewRunCmd(&client.Options{ConfigDir: t.TempDir()})
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs(args)
	return root.Cmd.Execute()
}

func TestAcceptAsksFirst(t *testing.T) {
	err := execRun(t, "accept", testRunID, "--reason", "fixing forward")
	require.EqualError(t, err, "--yes required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	err = execRun(t, "accept", testRunID, "--reason", "fixing forward", "--yes")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "accepted, and stopped only at sign-in")
}

func TestListHasNoStatusFilter(t *testing.T) {
	require.ErrorContains(t, execRun(t, "list", "--status", "failed"), "unknown flag: --status")
}
