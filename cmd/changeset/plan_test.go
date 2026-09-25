package changeset

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func TestPlanArguments(t *testing.T) {
	_, err := run(t, "plan")
	requireUsage(t, err, "missing argument: plan <change-set>")

	_, err = run(t, "plan", "cs-1")
	requireUsage(t, err, `invalid change set "cs-1"`)

	_, err = run(t, "plan", csID, "api")
	requireUsage(t, err, "accepts 1 arg(s), received 2")

	_, err = run(t, "plan", csID, "--wait-timeout", "0s")
	requireUsage(t, err, "--wait-timeout must be positive")

	_, err = run(t, "plan", csID, "--plan")
	require.ErrorContains(t, err, "unknown flag: --plan")

	for _, args := range [][]string{
		{"plan", csID},
		{"plan", csID, "--no-wait"},
		{"plan", csID, "--if-revision", "4", "--wait-timeout", "30s"},
	} {
		_, err := run(t, args...)
		requireAccepted(t, err, args...)
	}
}

// --plan is on every verb that cuts a revision, and the wait flags mean
// nothing without it.
func TestPlanFlagOnEachEditVerb(t *testing.T) {
	values := downloaded(t, valuesfileHeader(3), map[string]any{})
	verbs := [][]string{
		{"create", "shop/prod", "--set", "api.replicas=3"},
		{"add", csID, "users-db", "--from", "cloud-sql:v1"},
		{"set", csID, "api.replicas=3"},
		{"set", csID, "api", "--to", "v1.4.0"},
		{"unset", csID, "api.replicas"},
		{"remove", csID, "api"},
		{"values", csID, "api", "--values", values},
	}
	for _, args := range verbs {
		for _, extra := range [][]string{{"--plan"}, {"--plan", "--no-wait"}, {"--plan", "--wait-timeout", "1m"}} {
			a := append(append([]string{}, args...), extra...)
			_, err := run(t, a...)
			requireAccepted(t, err, a...)
		}
		for _, extra := range []string{"--no-wait", "--wait-timeout=1m"} {
			a := append(append([]string{}, args...), extra)
			_, err := run(t, a...)
			requireUsage(t, err, "--no-wait and --wait-timeout need --plan")
		}
	}

	_, err := run(t, "create", "shop/prod", "--plan")
	requireUsage(t, err, "--plan needs a revision")

	_, err = run(t, "values", csID, "api", "--plan")
	requireUsage(t, err, "--plan applies to an upload with --values")
}

func prep(s changesetv1.PrepareStatus) *changesetv1.Prepare {
	return &changesetv1.Prepare{Revision: 4, Status: s}
}

// fastPoll makes the waits below take milliseconds.
func fastPoll(t *testing.T) {
	t.Helper()
	first, maxp := pollFirst, pollMax
	pollFirst, pollMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { pollFirst, pollMax = first, maxp })
}

func TestWaitPrepare(t *testing.T) {
	fastPoll(t)
	seq := func(ss ...changesetv1.PrepareStatus) func(context.Context) (*changesetv1.Prepare, error) {
		return func(context.Context) (*changesetv1.Prepare, error) {
			s := ss[0]
			if len(ss) > 1 {
				ss = ss[1:]
			}
			return prep(s), nil
		}
	}
	var moves []changesetv1.PrepareStatus
	changed := func(p *changesetv1.Prepare) { moves = append(moves, p.Status) }

	got, err := waitPrepare(context.Background(), prep(changesetv1.PrepareStatus_QUEUED), time.Second,
		seq(changesetv1.PrepareStatus_QUEUED, changesetv1.PrepareStatus_RUNNING, changesetv1.PrepareStatus_PREPARED), changed)
	require.NoError(t, err)
	assert.Equal(t, changesetv1.PrepareStatus_PREPARED, got.Status)
	assert.Equal(t, []changesetv1.PrepareStatus{changesetv1.PrepareStatus_RUNNING, changesetv1.PrepareStatus_PREPARED}, moves,
		"only a move is reported")

	_, err = waitPrepare(context.Background(), prep(changesetv1.PrepareStatus_RUNNING), 20*time.Millisecond,
		seq(changesetv1.PrepareStatus_RUNNING), changed)
	err = prepareTimeout(err, csID, 4)
	assert.Equal(t, cmderr.ExitTimeout, cmderr.Code(err))
	assert.Contains(t, err.Error(), "revision 4 is still running")
	assert.Contains(t, cmderr.Hint(err), "admiral changeset plan "+csID+" --if-revision 4")

	boom := errors.New("boom")
	_, err = waitPrepare(context.Background(), prep(changesetv1.PrepareStatus_QUEUED), time.Second,
		func(context.Context) (*changesetv1.Prepare, error) { return nil, boom }, changed)
	assert.ErrorIs(t, prepareTimeout(err, csID, 4), boom, "a failed read is not a timeout")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = waitPrepare(ctx, prep(changesetv1.PrepareStatus_QUEUED), time.Second, seq(changesetv1.PrepareStatus_QUEUED), changed)
	assert.ErrorIs(t, err, context.Canceled)
}

// A prepare that fails or is superseded exits 1; one still queued under
// --no-wait, or prepared, exits 0.
func TestPrepareOutcome(t *testing.T) {
	assert.NoError(t, prepareOutcome(csID, prep(changesetv1.PrepareStatus_PREPARED)))
	assert.NoError(t, prepareOutcome(csID, prep(changesetv1.PrepareStatus_QUEUED)))

	failed := prep(changesetv1.PrepareStatus_FAILED)
	failed.Error = &changesetv1.PrepareError{Class: changesetv1.PrepareErrorClass_RENDER, Component: "api", Message: "template: bad"}
	err := prepareOutcome(csID, failed)
	assert.Equal(t, cmderr.ExitError, cmderr.Code(err))
	assert.EqualError(t, err, csID+" revision 4 failed to prepare (render error): api: template: bad")
	assert.Contains(t, cmderr.Hint(err), "same result")

	failed.Error = &changesetv1.PrepareError{Class: changesetv1.PrepareErrorClass_INFRA, Message: "storage unavailable"}
	err = prepareOutcome(csID, failed)
	assert.EqualError(t, err, csID+" revision 4 failed to prepare (infra error): storage unavailable")
	assert.Contains(t, cmderr.Hint(err), "queue it again")

	err = prepareOutcome(csID, prep(changesetv1.PrepareStatus_SUPERSEDED))
	assert.Equal(t, cmderr.ExitError, cmderr.Code(err))
	assert.Contains(t, err.Error(), "superseded")
	assert.Contains(t, cmderr.Hint(err), "admiral changeset plan "+csID)
}

func TestPrintPrepare(t *testing.T) {
	var b bytes.Buffer
	p := testPrinter(&b)
	pr := prep(changesetv1.PrepareStatus_PREPARED)
	pr.ArtifactDigest = "sha256:3f9a2c1d0e4b5a6978"
	pr.Findings = []*changesetv1.Finding{
		{Component: "api", Code: changesetv1.FindingCode_LOOKUP_USED, Message: "the chart calls lookup"},
		{Component: "web", Code: changesetv1.FindingCode_NONDETERMINISTIC, Message: "renders differ",
			Details: []string{"Secret/web/tls data.key", "Deployment/web/web spec.template"}},
	}
	require.NoError(t, printPrepare(p, pr))
	out := b.String()
	assert.Regexp(t, `4\s+prepared\s+sha256:3f9a2c1d0e4b\s+2`, out)
	assert.Regexp(t, `COMPONENT\s+CODE\s+MESSAGE\s+DETAILS`, out)
	assert.Regexp(t, `api\s+lookup-used\s+the chart calls lookup\s+<none>`, out)
	assert.Contains(t, out, "Secret/web/tls data.key; Deployment/web/web spec.template")
}

func testPrinter(w *bytes.Buffer) *output.Printer {
	return &output.Printer{Format: output.FormatTable, IO: iostreams.New(nil, w, w, func(string) string { return "" })}
}

func valuesfileHeader(rev int32) valuesfile.Header {
	return valuesfile.Header{ChangeSet: csID, Component: "api", Revision: rev}
}
