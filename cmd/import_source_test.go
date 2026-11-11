package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/admiraltest"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
)

// readySource is the shape whose enums land on READY and GIT, so the mock's
// source can be imported through.
func readySource(t *testing.T) admiraltest.Shape {
	t.Helper()
	for _, s := range admiraltest.Shapes() {
		if s.String() == "branch-1" {
			return s
		}
	}
	t.Fatal("no branch-1 shape")
	return admiraltest.Shape{}
}

// Through a source the version is never guessed: without a terminal the
// refusal lists what the source offers; in one, the list is asked and an
// empty answer imports nothing.
func TestImportThroughASourceNeverGuessesTheVersion(t *testing.T) {
	t.Setenv(credentials.EnvAPIKey, "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG")
	srv := admiraltest.Start(t)
	srv.Respond(readySource(t))

	r := sweepRun(t, srv.Addr(), []string{"component", "import", "--from", "src-000000000001", "cert-manager"})
	require.Error(t, r.err)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(r.err))
	require.Contains(t, r.err.Error(), "--version required when not running interactively")
	require.True(t, strings.HasPrefix(cmderr.Hint(r.err), "Pass --version as one of: "), cmderr.Hint(r.err))

	r = sweepRun(t, srv.Addr(), []string{"component", "import", "--from", "src-000000000001", "cert-manager", "--version", "latest"})
	require.NoError(t, r.err, r.stderr)
	require.Contains(t, r.stderr, "Importing cert-manager latest from")
}

func TestImportThroughASourceAsksInATerminal(t *testing.T) {
	t.Setenv(credentials.EnvAPIKey, "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG")
	t.Setenv("ADMIRAL_FORCE_INTERACTIVE", "1")
	srv := admiraltest.Start(t)
	srv.Respond(readySource(t))

	r := interactiveRun(t, srv.Addr(), "\n", []string{"component", "import", "--from", "src-000000000001", "cert-manager"})
	require.Error(t, r.err)
	require.Contains(t, r.stderr, "Choose the version [number or name]: ")
	require.Contains(t, r.stderr, "canceled")
	require.NotContains(t, r.stderr, "Importing")

	r = interactiveRun(t, srv.Addr(), "1\n", []string{"component", "import", "--from", "src-000000000001", "cert-manager"})
	require.NoError(t, r.err, r.stderr)
	require.Contains(t, r.stderr, "Importing cert-manager ")
}
