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
	tags, commit, skipped := defaultTags(t.Context(), root)
	assert.Equal(t, []string{"main"}, tags)
	assert.Equal(t, "sha-"+sha, commit)
	assert.Empty(t, skipped)

	// CI checks out a detached HEAD and names the branch in the environment.
	require.NoError(t, exec.Command("git", "-C", root, "checkout", "-q", "--detach").Run())
	t.Setenv("GITHUB_REF_NAME", "master")
	tags, commit, skipped = defaultTags(t.Context(), root)
	assert.Equal(t, []string{"master"}, tags)
	assert.Equal(t, "sha-"+sha, commit)
	assert.Empty(t, skipped)

	// A ref with a slash (feature/x) is not a tag name; only the sha is
	// applied, and the caller is told which branch was passed over.
	t.Setenv("GITHUB_REF_NAME", "feature/x")
	tags, commit, skipped = defaultTags(t.Context(), root)
	assert.Empty(t, tags)
	assert.Equal(t, "sha-"+sha, commit)
	assert.Equal(t, "feature/x", skipped)

	// Outside git there is nothing to derive; --tag is the caller's job.
	t.Setenv("GITHUB_REF_NAME", "")
	tags, commit, skipped = defaultTags(t.Context(), t.TempDir())
	assert.Empty(t, tags)
	assert.Empty(t, commit)
	assert.Empty(t, skipped)
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

// Only what the registry treats as immutable is applied as a chart's version
// tag; a chart that calls itself "latest" or "1.0" gets no version tag.
func TestSemverPattern(t *testing.T) {
	for _, ok := range []string{"0.1.0", "v1.2.3", "9.5.1", "1.0.0-rc.1", "1.0.0+build.7"} {
		assert.True(t, semverPattern.MatchString(ok), ok)
	}
	for _, bad := range []string{"", "latest", "1.0", "1.0.0.0", "v1", "01.0.0"} {
		assert.False(t, semverPattern.MatchString(bad), bad)
	}
}
