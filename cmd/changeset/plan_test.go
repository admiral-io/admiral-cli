package changeset

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/valuesfile"
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
		{"remove", csID, "api", "--orphan"},
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

func valuesfileHeader(rev int32) valuesfile.Header {
	return valuesfile.Header{ChangeSet: csID, Component: "api", Revision: rev}
}
