package changeset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func valueConflict(path ...string) *changesetv1.Conflict {
	return &changesetv1.Conflict{Component: "api", Kind: changesetv1.Conflict_KIND_VALUE, Path: pathSegments(path),
		ChangeSetValueJson: `"1.5"`, EnvironmentValueJson: `"1.4"`, EnvironmentChangeSetId: "cs-aaaaaaaaaaaa"}
}

// Every conflict needs a flag, and a value is named as --set names it, a
// version or a removal by the bare component.
func TestRebaseResolvesEachConflictByItsFlag(t *testing.T) {
	conflicts := []*changesetv1.Conflict{
		valueConflict("image", "tag"),
		{Component: "web", Kind: changesetv1.Conflict_KIND_VERSION},
		{Component: "worker", Kind: changesetv1.Conflict_KIND_REMOVED},
	}
	keeps, err := parseKeeps([]string{"api.image.tag"}, []string{"web", "worker"})
	require.NoError(t, err)

	got, err := resolveConflicts(csID, conflicts, keeps)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, changesetv1.Resolution_KEEP_CHANGE_SET, got[0].Keep)
	assert.Equal(t, []string{"image", "tag"}, keys(got[0].Path))
	assert.Equal(t, changesetv1.Conflict_KIND_VERSION, got[1].Kind)
	assert.Equal(t, changesetv1.Resolution_KEEP_ENVIRONMENT, got[1].Keep)
	assert.Equal(t, changesetv1.Conflict_KIND_REMOVED, got[2].Kind)
}

// An unresolved conflict is listed with the flags that settle it, before
// the rebase is sent.
func TestRebaseListsAConflictLeftOpen(t *testing.T) {
	_, err := resolveConflicts(csID, []*changesetv1.Conflict{valueConflict("image", "tag")}, nil)
	requireUsage(t, err, "has 1 conflict(s) with its environment")
	hint := cmderr.Hint(err)
	assert.Contains(t, hint, `api.image.tag: changeset "1.5", environment "1.4"`)
	assert.Contains(t, hint, "--keep-changeset api.image.tag  or  --keep-environment api.image.tag")
	assert.Contains(t, hint, "(environment side from cs-aaaaaaaaaaaa)")
}

func TestRebaseRefusesAFlagThatNamesNoConflict(t *testing.T) {
	keeps, err := parseKeeps([]string{"api.replicas"}, nil)
	require.NoError(t, err)
	_, err = resolveConflicts(csID, []*changesetv1.Conflict{valueConflict("image", "tag")}, keeps)
	requireUsage(t, err, "--keep-changeset api.replicas names no conflict")

	// A bare component is its version, not every value under it.
	keeps, err = parseKeeps([]string{"api"}, nil)
	require.NoError(t, err)
	_, err = resolveConflicts(csID, []*changesetv1.Conflict{valueConflict("image", "tag")}, keeps)
	requireUsage(t, err, "--keep-changeset api names no conflict")
}

func TestRebaseKeepsARemovalOnlyForTheEnvironment(t *testing.T) {
	keeps, err := parseKeeps([]string{"worker"}, nil)
	require.NoError(t, err)
	_, err = resolveConflicts(csID, []*changesetv1.Conflict{{Component: "worker", Kind: changesetv1.Conflict_KIND_REMOVED}}, keeps)
	requireUsage(t, err, "only --keep-environment worker resolves it")
}

func TestRebaseFlagsAreCheckedBeforeTheNetwork(t *testing.T) {
	_, err := run(t, "rebase", csID, "--keep-changeset", "api.image.tag", "--keep-environment", "api.image.tag")
	requireUsage(t, err, "api.image.tag is resolved twice")

	_, err = run(t, "rebase", csID, "--keep-changeset", "api.image.tag=v2")
	requireUsage(t, err, "name a component or component.path, not a value")

	_, err = run(t, "rebase", csID, "--no-wait")
	requireUsage(t, err, "need --plan")

	_, err = run(t, "rebase", csID, "--keep-changeset", "api.image.tag", "--plan")
	requireAccepted(t, err, "rebase", csID)
}
