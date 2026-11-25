package source

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// run executes the command with an empty, non-terminal stdin, so every
// prompt is an error naming its flag.
func run(t *testing.T, args ...string) error {
	t.Helper()

	root := NewSourceCmd(&client.Options{})
	root.Cmd.SetIn(strings.NewReader(""))
	root.Cmd.SetOut(io.Discard)
	root.Cmd.SetErr(io.Discard)
	root.Cmd.SetArgs(args)

	return root.Cmd.Execute()
}

// What can be refused is refused before any network, as a usage error that
// says what to pass.
func TestSourceRefusesLocallyWhatItCan(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"create"}, "missing argument"},
		{[]string{"create", "argo"}, "name the location with one of --helm, --oci, --terraform or --git"},
		{[]string{"create", "argo", "--helm", "https://a", "--git", "https://b"}, "a source is one location; pass one of --helm, --git"},
		{[]string{"create", "argo", "--helm", " "}, "--helm needs a location"},
		{[]string{"create", "argo", "--helm", "https://a", "--credential", "c", "--integration", "i"}, "--credential or --integration, not both"},
		{[]string{"update", "argo"}, "at least one of"},
		{[]string{"update", "argo", "--credential", "c", "--clear-grant"}, "pass one of --credential, --integration or --clear-grant"},
		{[]string{"delete", "argo"}, "--yes required when not running interactively"},
		{[]string{"search"}, "missing argument"},
		{[]string{"search", "a", "b", "c"}, "accepts at most 2 arg(s)"},
		{[]string{"get"}, "missing argument"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := run(t, tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
		})
	}
}

// A cell never reads as a missing value: an open source says it allows
// everything, and a public one says it holds no grant.
func TestCellsSayWhatEmptyMeans(t *testing.T) {
	assert.Equal(t, "* (everything)", formatAllows([]string{"*"}))
	assert.Equal(t, "argo-rollouts,cert-*", formatAllows([]string{"argo-rollouts", "cert-*"}))
	assert.Equal(t, "none", formatGrant(&sourcev1.Source{}))
	assert.Equal(t, "credential cred-1", formatGrant(&sourcev1.Source{Grant: &sourcev1.Source_CredentialId{CredentialId: "cred-1"}}))
	assert.Equal(t, "needs-attention: gone", formatStatus(&sourcev1.Source{Status: sourcev1.Source_STATUS_NEEDS_ATTENTION, StatusReason: "gone"}))
	assert.Equal(t, "helm", kindName(sourcev1.Source_KIND_HELM_REPO))
}
