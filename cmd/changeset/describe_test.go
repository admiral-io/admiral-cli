package changeset

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/cmderr"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// The changeset that sent the user from env describe to cs get and cs diff:
// open, one image tag changed, and its run failed. describe shows all three
// and says what to run next.
func TestDescribeShowsTheChangeAndTheFailedRun(t *testing.T) {
	ago := timestamppb.New(time.Now().Add(-10 * time.Hour))
	cs := &changesetv1.ChangeSet{
		Id: csID, Title: "admiral-platform 35bd16b", Status: changesetv1.ChangeSet_STATUS_OPEN,
		HeadRevision: 1, CreatedAt: ago, LatestRunId: "run-7zd93c9p4qer",
	}
	sec := csSections{
		where: "admiral-platform/dev",
		diff: &changesetv1.DiffChangeSetResponse{Revision: 1, Components: []*changesetv1.ComponentDiff{{
			Component: "admiral-platform", Action: changesetv1.EntryAction_ENTRY_ACTION_UPDATE,
			OldVersion: &changesetv1.VersionRef{Digest: "sha256:ced3cf345b3c0000"},
			NewVersion: &changesetv1.VersionRef{Digest: "sha256:ced3cf345b3c0000"},
			Paths: []*changesetv1.PathDiff{{
				DisplayPath: "image.tag", BeforePresent: true, BeforeJson: `"cea12b5"`, AfterPresent: true, AfterJson: `"35bd16b"`,
			}},
		}}},
		runs: []*runv1.Run{{Id: "run-7zd93c9p4qer", Status: runv1.Run_STATUS_FAILED, CreatedAt: ago}},
	}
	var b bytes.Buffer
	require.NoError(t, describeChangeSet(cs, sec).Render(&b))
	out := b.String()
	assert.Contains(t, out, "Environment:  admiral-platform/dev")
	assert.Contains(t, out, "Last Run:     run-7zd93c9p4qer Failed 10h ago")
	assert.Regexp(t, `admiral-platform\s+update\s+sha256:ced3cf345b3c\s+image.tag: "cea12b5" -> "35bd16b"`, out)
	assert.Contains(t, out, "\nRuns:\n")
	assert.Contains(t, out, "To see why the last run failed, run: admiral run get run-7zd93c9p4qer")
	assert.Contains(t, out, "To see every value, run: admiral changeset diff "+csID)
	assert.NotContains(t, out, "To apply it")
}

func TestChangesInBriefCountsWhatItCannotFit(t *testing.T) {
	leaf := func(path string) *changesetv1.PathDiff {
		return &changesetv1.PathDiff{DisplayPath: path, AfterPresent: true, AfterJson: `1`}
	}
	assert.Equal(t, "replicas: <absent> -> 1", changesInBrief(&changesetv1.ComponentDiff{Paths: []*changesetv1.PathDiff{leaf("replicas")}}))
	assert.Equal(t, "2 values, 1 violation", changesInBrief(&changesetv1.ComponentDiff{
		Paths:      []*changesetv1.PathDiff{leaf("a"), leaf("b")},
		Violations: []*changesetv1.Violation{{Path: "a", Message: "no"}},
	}))
	long := &changesetv1.PathDiff{DisplayPath: "config.database.url", BeforePresent: true,
		BeforeJson: `"postgres://a-very-long-host.example.com/db"`, AfterPresent: true, AfterJson: `"postgres://another.example.com/db"`}
	assert.Equal(t, "1 value", changesInBrief(&changesetv1.ComponentDiff{Paths: []*changesetv1.PathDiff{long}}))
}

// A section the credential cannot read is unavailable; the rest still shows.
func TestDescribeWithoutItsRunsStillShowsTheChange(t *testing.T) {
	cs := &changesetv1.ChangeSet{Id: csID, Status: changesetv1.ChangeSet_STATUS_OPEN, LatestRunId: "run-000000000001"}
	sec := csSections{
		where: "shop/prod",
		diff: &changesetv1.DiffChangeSetResponse{Components: []*changesetv1.ComponentDiff{{
			Component: "api", Action: changesetv1.EntryAction_ENTRY_ACTION_CREATE,
		}}},
		runsErr: status.Error(codes.PermissionDenied, "needs run:read"),
	}
	var b bytes.Buffer
	require.NoError(t, describeChangeSet(cs, sec).Render(&b))
	out := b.String()
	assert.Contains(t, out, "Last Run:     run-000000000001")
	assert.Regexp(t, `api\s+create`, out)
	assert.Contains(t, out, "Runs:")
	assert.Contains(t, out, "unavailable")
	assert.Contains(t, out, cmderr.ScopeHint)
}

// The platform lists runs only within an environment, so describe names the
// changeset's environment along with the changeset.
func TestDescribeListsRunsWithinTheChangesetsEnvironment(t *testing.T) {
	f, err := changeSetRunsFilter(&changesetv1.ChangeSet{Id: csID, EnvironmentId: "env-dcd44upzgg4z"})
	require.NoError(t, err)
	assert.Equal(t, "field['environment_id'] = 'env-dcd44upzgg4z' AND field['change_set_id'] = '"+csID+"'", f)

	hints := nextSteps(&changesetv1.ChangeSet{Id: csID}, nil, csSections{where: "shop/prod", moreRuns: true})
	assert.Contains(t, hints, "Showing the 5 most recent runs. To list them all, run: admiral run list shop/prod --changeset "+csID)
}
