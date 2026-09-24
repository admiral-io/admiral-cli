package changeset

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

func buildArtifact(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		tf := e.typeflag
		if tf == 0 {
			tf = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tf, Linkname: e.linkname}
		if tf != tar.TypeReg {
			h.Size = 0
		}
		require.NoError(t, tw.WriteHeader(h))
		if tf == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// fixtureArtifact is two components, one with a finding and hooks, the
// shape the worker writes.
func fixtureArtifact(t *testing.T) []byte {
	return buildArtifact(t,
		tarEntry{name: "artifact.json", body: `{"schema":1,"components":[` +
			`{"name":"api","namespace":"shop-prod","kind":"workload"},` +
			`{"name":"worker","namespace":"jobs"}]}`},
		tarEntry{name: "components/", typeflag: tar.TypeDir},
		tarEntry{name: "components/api/findings.json", body: `[{"component":"api","code":"LOOKUP_USED",` +
			`"message":"the chart calls lookup,\nwhich sees no cluster","details":["ConfigMap/shop-prod/api data"]}]`},
		tarEntry{name: "components/api/hooks.yaml", body: "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: migrate\n"},
		tarEntry{name: "components/api/manifests.yaml", body: "apiVersion: v1\nkind: Secret\nmetadata:\n  name: api\n" +
			"data:\n  password: sha256:3f9a2c1d0e4b\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: api"},
		tarEntry{name: "components/worker/findings.json", body: `[]`},
		tarEntry{name: "components/worker/hooks.yaml", body: ""},
		tarEntry{name: "components/worker/manifests.yaml", body: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: worker\n"},
	)
}

func TestWriteRendered(t *testing.T) {
	files, err := readArtifact(fixtureArtifact(t))
	require.NoError(t, err)
	comps, err := components(files)
	require.NoError(t, err)

	var b bytes.Buffer
	writeRendered(&b, comps)
	assert.Equal(t, `# component: api  namespace: shop-prod
# finding: lookup-used: the chart calls lookup, which sees no cluster
#   ConfigMap/shop-prod/api data
apiVersion: v1
kind: Secret
metadata:
  name: api
data:
  password: sha256:3f9a2c1d0e4b
---
apiVersion: v1
kind: Service
metadata:
  name: api
---
# hooks
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
---
# component: worker  namespace: jobs
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker
`, b.String(), "a masked secret is printed as it arrived")

	one, err := onlyComponent(comps, "worker", csID, 4)
	require.NoError(t, err)
	b.Reset()
	writeRendered(&b, one)
	assert.NotContains(t, b.String(), "# component: api")
	assert.Contains(t, b.String(), "# component: worker  namespace: jobs")

	_, err = onlyComponent(comps, "web", csID, 4)
	assert.EqualError(t, err, `component "web" is not in `+csID+` revision 4, which renders: api, worker`)
}

// artifact.json may key its components by name; a missing namespace is
// shown as <none>, never guessed.
func TestRenderedNamespaces(t *testing.T) {
	ns, err := artifactNamespaces([]byte(`{"components":{"api":{"namespace":"shop-prod"}}}`))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"api": "shop-prod"}, ns)

	files, err := readArtifact(buildArtifact(t, tarEntry{name: "components/api/manifests.yaml", body: "kind: Service\n"}))
	require.NoError(t, err)
	comps, err := components(files)
	require.NoError(t, err)
	var b bytes.Buffer
	writeRendered(&b, comps)
	assert.Equal(t, "# component: api  namespace: <none>\nkind: Service\n", b.String())
}

// An entry that would land outside the output directory refuses the whole
// artifact, before anything is written.
func TestUnpackRefusesPathTraversal(t *testing.T) {
	for _, e := range []tarEntry{
		{name: "../evil.yaml", body: "x"},
		{name: "components/../../evil.yaml", body: "x"},
		{name: "/etc/evil.yaml", body: "x"},
		{name: `components\..\..\evil.yaml`, body: "x"},
		{name: "components/api/link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		{name: "components/api/hard", typeflag: tar.TypeLink, linkname: "../../evil"},
	} {
		t.Run(e.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "out")
			gz := buildArtifact(t, tarEntry{name: "artifact.json", body: "{}"}, e)
			files, err := readArtifact(gz)
			require.Error(t, err)
			assert.Contains(t, err.Error(), strconv.Quote(e.name))
			assert.Nil(t, files)
			_, statErr := os.Stat(dir)
			assert.ErrorIs(t, statErr, os.ErrNotExist, "nothing is written")
			_, statErr = os.Stat(filepath.Join(parent, "evil.yaml"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

// A symlink already in the directory cannot carry a write outside it.
func TestUnpackStaysInsideThroughASymlink(t *testing.T) {
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(outside, 0o755))
	dir := filepath.Join(parent, "out")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "components")))

	files, err := readArtifact(buildArtifact(t, tarEntry{name: "components/api/manifests.yaml", body: "kind: Service\n"}))
	require.NoError(t, err)
	require.Error(t, unpack(dir, files))
	_, err = os.Stat(filepath.Join(outside, "api", "manifests.yaml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestUnpack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	files, err := readArtifact(fixtureArtifact(t))
	require.NoError(t, err)
	require.NoError(t, unpack(dir, files))
	data, err := os.ReadFile(filepath.Join(dir, "components", "worker", "manifests.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: worker")
	_, err = os.Stat(filepath.Join(dir, "artifact.json"))
	assert.NoError(t, err)

	require.NoError(t, checkOutputDir(filepath.Join(t.TempDir(), "new"), false), "a missing directory is created")
	require.NoError(t, checkOutputDir(t.TempDir(), false), "an empty one is used")
	err = checkOutputDir(dir, false)
	requireUsage(t, err, "is not empty")
	assert.Contains(t, cmderr.Hint(err), "--force")
	assert.NoError(t, checkOutputDir(dir, true))
}

func TestGetRenderedArguments(t *testing.T) {
	full := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(full, "x"), nil, 0o644))

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get", csID, "--component", "api"}, "--component needs --rendered"},
		{[]string{"get", csID, "--revision", "3"}, "--revision needs --rendered"},
		{[]string{"get", csID, "--output-dir", "out"}, "--output-dir needs --rendered"},
		{[]string{"get", csID, "-f"}, "--force needs --rendered"},
		{[]string{"get", csID, "--rendered", "--revision", "0"}, "--revision must be 1 or more"},
		{[]string{"get", csID, "--rendered", "--component", "Api"}, `invalid component name "Api"`},
		{[]string{"get", csID, "--rendered", "--force"}, "--force applies to --output-dir"},
		{[]string{"get", csID, "--rendered", "--component", "api", "--output-dir", t.TempDir()}, "--component cannot be combined with --output-dir"},
		{[]string{"get", csID, "--rendered", "--output-dir", full}, "is not empty"},
	}
	for _, tc := range cases {
		_, err := run(t, tc.args...)
		requireUsage(t, err, tc.want)
	}

	for _, args := range [][]string{
		{"get", csID, "--rendered"},
		{"get", csID, "--rendered", "--revision", "3", "--component", "api"},
		{"get", csID, "--rendered", "--output-dir", full, "-f"},
		{"get", csID, "--rendered", "--output-dir", filepath.Join(full, "new")},
	} {
		_, err := run(t, args...)
		requireAccepted(t, err, args...)
	}
}

// get never prepares: with nothing prepared it names the command that does.
func TestPreparedRevision(t *testing.T) {
	require.NoError(t, preparedRevision(csID, &changesetv1.Prepare{Revision: 2, Status: changesetv1.PrepareStatus_PREPARED}))

	err := preparedRevision(csID, nil)
	assert.EqualError(t, err, csID+" has no prepared revision")
	assert.Equal(t, "Run 'admiral changeset plan "+csID+"' to prepare the head.", cmderr.Hint(err))
	assert.Equal(t, cmderr.ExitError, cmderr.Code(err))

	err = preparedRevision(csID, &changesetv1.Prepare{Revision: 2, Status: changesetv1.PrepareStatus_RUNNING})
	assert.EqualError(t, err, csID+" revision 2 is running, not yet prepared")
	assert.Contains(t, cmderr.Hint(err), "--if-revision 2")

	err = preparedRevision(csID, &changesetv1.Prepare{Revision: 2, Status: changesetv1.PrepareStatus_FAILED})
	assert.EqualError(t, err, csID+" revision 2 was not prepared: it failed")
	assert.Contains(t, cmderr.Hint(err), "admiral changeset plan "+csID)
}
