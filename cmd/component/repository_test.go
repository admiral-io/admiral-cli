package component

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

// gitRepo makes a one-commit repository on branch main.
func gitRepo(t *testing.T) (root, sha string) {
	t.Helper()
	root = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "agent", "main.tf"), []byte(`variable "a" {}`), 0o644))
	run("init", "-q", "-b", "main")
	run("add", "-A")
	run("commit", "-qm", "one")
	sha = run("rev-parse", "--short=7", "HEAD")
	return root, sha[:7]
}

func TestDefaultTagsAreBranchAndSha(t *testing.T) {
	root, sha := gitRepo(t)
	t.Setenv("GITHUB_REF_NAME", "")
	assert.Equal(t, []string{"main", "sha-" + sha}, defaultTags(t.Context(), root))

	// CI checks out a detached HEAD and names the branch in the environment.
	require.NoError(t, exec.Command("git", "-C", root, "checkout", "-q", "--detach").Run())
	t.Setenv("GITHUB_REF_NAME", "master")
	assert.Equal(t, []string{"master", "sha-" + sha}, defaultTags(t.Context(), root))

	// A ref with a slash (feature/x) is not a tag name; only the sha is applied.
	t.Setenv("GITHUB_REF_NAME", "feature/x")
	assert.Equal(t, []string{"sha-" + sha}, defaultTags(t.Context(), root))

	// Outside git there is nothing to derive; --tag is the caller's job.
	assert.Empty(t, defaultTags(t.Context(), t.TempDir()))
}

// The thing pointed at says what it is. A directory with admiral.yaml is a
// repository, and the per-component flags do not apply to it.
func TestRepositoryPublishUsageErrors(t *testing.T) {
	root, _ := gitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "admiral.yaml"), []byte("components:\n  - path: modules/agent\n"), 0o644))

	_, err := run(t, "publish", root, "--name", "x")
	require.Error(t, err)
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	assert.Contains(t, err.Error(), "--name and --kind apply to one component")

	_, err = run(t, "publish", root, "-f", filepath.Join(root, "admiral.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")

	// A manifest that is not there is an error naming the path, not a scan.
	_, err = run(t, "publish", "-f", filepath.Join(root, "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope.yaml")
}
