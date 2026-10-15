package changeset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// An add with --set is one request: the component first, then each set
// on it, with paths that start inside the component.
func TestAddEditsPutSetsAfterTheComponent(t *testing.T) {
	src := &changesetv1.RegistryRef{Name: "hello", Reference: "v0.1.0"}
	es, err := addEdits("hello", src, map[string]any{"replicas": 2},
		[]string{"message=hello from dev", `annotations."a.b"=x`}, []string{"build=0042"})
	require.NoError(t, err)
	require.Len(t, es, 4)

	add := es[0].GetAddComponent()
	require.Equal(t, "hello", add.GetName())
	require.JSONEq(t, `{"replicas":2}`, add.GetValuesJson())

	msg := es[1].GetSetValue()
	require.Equal(t, "hello", msg.GetComponent())
	require.Equal(t, []string{"message"}, keys(msg.GetPath()))
	require.JSONEq(t, `"hello from dev"`, msg.GetValueJson())

	require.Equal(t, []string{"annotations", "a.b"}, keys(es[2].GetSetValue().GetPath()))

	// --set-string comes last, as helm applies it, and stays a string.
	build := es[3].GetSetValue()
	require.Equal(t, []string{"build"}, keys(build.GetPath()))
	require.JSONEq(t, `"0042"`, build.GetValueJson())
}

// A path that repeats the component's name is a key of that name inside
// it, not the component: --set on add never takes the set form.
func TestAddSetPathStartsInsideTheComponent(t *testing.T) {
	src := &changesetv1.RegistryRef{Name: "api", Reference: "v1"}
	es, err := addEdits("api", src, nil, []string{"api.port=8080"}, nil)
	require.NoError(t, err)
	require.Empty(t, es[0].GetAddComponent().GetValuesJson(), "no file, no values")
	require.Equal(t, []string{"api", "port"}, keys(es[1].GetSetValue().GetPath()))
}

func TestAddSetWithoutAValueIsUsage(t *testing.T) {
	_, err := run(t, "add", csID, "hello:v0.1.0", "--set", "message")
	requireUsage(t, err, "is not component.path=value")
}

// --values - reads stdin, and is parsed before any sign-in.
func TestAddValuesFromStdin(t *testing.T) {
	root := newTestRoot(t)
	root.Cmd.SetIn(strings.NewReader("message: [unclosed\n"))
	root.Cmd.SetArgs([]string{"add", csID, "hello:v0.1.0", "--values", "-"})
	requireUsage(t, root.Cmd.Execute(), "-:")

	root = newTestRoot(t)
	root.Cmd.SetIn(strings.NewReader("message: hello from dev\n"))
	root.Cmd.SetArgs([]string{"add", csID, "hello:v0.1.0", "--values", "-"})
	requireAccepted(t, root.Cmd.Execute(), "add", "--values", "-")
}
