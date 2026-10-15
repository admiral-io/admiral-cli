package changeset

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

const runID = "run-m89nt2241g09"

func latestWith(t *testing.T, cs *changesetv1.ChangeSet, r *runv1.Run) (*runv1.Run, error) {
	t.Helper()
	c := &fakeClient{cs: &changeSetClient{got: cs}, runs: &runClient{run: r}}
	_, got, err := latestPlan(context.Background(), c, csID)
	return got, err
}

func TestLatestPlanIsThePlannedRun(t *testing.T) {
	r, err := latestWith(t, &changesetv1.ChangeSet{Id: csID, LatestRunId: runID},
		&runv1.Run{Id: runID, Status: runv1.Run_STATUS_PLANNED})
	require.NoError(t, err)
	require.Equal(t, runID, r.GetId())
}

// Each state that cannot be approved or applied says what to do instead.
func TestLatestPlanRefusesWhatIsNotWaitingOnAPlan(t *testing.T) {
	_, err := latestWith(t, &changesetv1.ChangeSet{Id: csID}, nil)
	require.EqualError(t, err, csID+" has not been planned")
	require.Contains(t, cmderr.Hint(err), "admiral changeset plan "+csID)

	cases := []struct {
		status runv1.Run_Status
		want   string
		hint   string
	}{
		{runv1.Run_STATUS_PLANNING, "is still planning", "admiral run get " + runID + " --wait"},
		{runv1.Run_STATUS_NO_CHANGES, "found nothing to change", ""},
		{runv1.Run_STATUS_FAILED, "latest plan failed", "admiral changeset plan " + csID},
		{runv1.Run_STATUS_APPLIED, "is applied, not waiting on a plan", ""},
	}
	for _, tc := range cases {
		_, err := latestWith(t, &changesetv1.ChangeSet{Id: csID, LatestRunId: runID},
			&runv1.Run{Id: runID, Status: tc.status})
		require.ErrorContains(t, err, tc.want, tc.status.String())
		if tc.hint != "" {
			require.Contains(t, cmderr.Hint(err), tc.hint, tc.status.String())
		}
	}
}

// What is shown before the question names the changeset, its author and
// the run, then the diff resource by resource.
func TestShowPlan(t *testing.T) {
	c := &fakeClient{runs: &runClient{diff: &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{{
		Component: "hello",
		Resource:  &agentv1.ResourceRef{Kind: "Deployment", Name: "hello", Namespace: "shop-dev"},
		Diff:      "+ replicas: 1",
	}}}}}
	var w bytes.Buffer
	err := showPlan(context.Background(), &w, c,
		&changesetv1.ChangeSet{Id: csID, Title: "First deploy"}, &runv1.Run{Id: runID})
	require.NoError(t, err)
	require.Contains(t, w.String(), csID+`  "First deploy"`)
	require.Contains(t, w.String(), "("+runID+")")
	require.Contains(t, w.String(), "# hello: shop-dev/Deployment/hello\n+ replicas: 1")
}

// Both verbs check their argument and whether they can ask before any
// sign-in; --yes is what a script passes.
func TestApproveAndApplyCheckBeforeTheNetwork(t *testing.T) {
	for _, verb := range []string{"approve", "apply"} {
		_, err := run(t, verb, "nope")
		requireUsage(t, err, `invalid changeset "nope"`)

		_, err = run(t, verb, "shop/dev")
		requireUsage(t, err, "--yes required when not running interactively")

		for _, ref := range []string{csID, "shop/dev"} {
			_, err = run(t, verb, ref, "--yes")
			requireAccepted(t, err, verb, ref)
		}
	}
	_, err := run(t, "apply", csID, "--yes", "--wait-timeout", "0s")
	requireUsage(t, err, "--wait-timeout must be positive")
}
