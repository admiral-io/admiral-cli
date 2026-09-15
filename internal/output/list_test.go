package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.admiral.io/cli/internal/iostreams"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func items(names ...string) []proto.Message {
	out := make([]proto.Message, len(names))
	for i, n := range names {
		out[i] = structpb.NewStringValue(n)
	}
	return out
}

func nameOf(l List) func(int) string {
	return func(i int) string { return l.Items[i].(*structpb.Value).GetStringValue() }
}

func TestPrintList_Table(t *testing.T) {
	var out, errOut bytes.Buffer
	p := testPrinterErr(FormatTable, &out, &errOut)
	l := List{Kind: "widgets", Items: items("a", "b")}
	err := p.PrintList(l, func(w *tabwriter.Writer) {
		Writeln(w, "NAME\tAGE")
		Writeln(w, "a\t1s")
		Writeln(w, "b\t2s")
	})
	require.NoError(t, err)
	require.Equal(t, "NAME   AGE\na      1s\nb      2s\n", out.String())
	require.Empty(t, errOut.String())
}

func TestPrintList_EmptyTableGoesToStderr(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatWide} {
		var out, errOut bytes.Buffer
		p := testPrinterErr(f, &out, &errOut)
		err := p.PrintList(List{Kind: "runs", Scope: "shop/prod"}, func(w *tabwriter.Writer) {
			t.Fatal("table renderer must not run for an empty list")
		})
		require.NoError(t, err)
		require.Empty(t, out.String(), "stdout stays empty so pipelines see nothing")
		require.Equal(t, "No runs found in shop/prod.\n", errOut.String())
	}

	var out, errOut bytes.Buffer
	p := testPrinterErr(FormatTable, &out, &errOut)
	require.NoError(t, p.PrintList(List{Kind: "applications"}, nil))
	require.Equal(t, "No applications found.\n", errOut.String())
}

func TestPrintList_JSONIsBareArray(t *testing.T) {
	var out bytes.Buffer
	p := testPrinter(FormatJSON, &out)
	require.NoError(t, p.PrintList(List{Kind: "w", Items: items("a", "b")}, nil))
	require.Equal(t, "[\"a\",\"b\"]\n", out.String(), "piped JSON is compact")

	var arr []string
	require.NoError(t, json.Unmarshal(out.Bytes(), &arr))
	require.Equal(t, []string{"a", "b"}, arr)
}

func TestPrintList_JSONEmptyIsEmptyArray(t *testing.T) {
	var out, errOut bytes.Buffer
	p := testPrinterErr(FormatJSON, &out, &errOut)
	require.NoError(t, p.PrintList(List{Kind: "w"}, nil))
	require.Equal(t, "[]\n", out.String())
	require.Empty(t, errOut.String(), "no human message in machine mode")
}

func TestPrintList_JSONIndentedOnTTY(t *testing.T) {
	var out bytes.Buffer
	p := &Printer{
		Format: FormatJSON,
		IO:     iostreams.New(strings.NewReader(""), &out, &bytes.Buffer{}, func(k string) string { return map[string]string{"ADMIRAL_FORCE_TTY": "100"}[k] }),
	}
	require.NoError(t, p.PrintList(List{Kind: "w", Items: items("a")}, nil))
	require.Equal(t, "[\n  \"a\"\n]\n", out.String())
}

func TestPrintList_YAML(t *testing.T) {
	var out bytes.Buffer
	p := testPrinter(FormatYAML, &out)
	require.NoError(t, p.PrintList(List{Kind: "w", Items: items("a", "b")}, nil))
	require.Equal(t, "- a\n- b\n", out.String())

	out.Reset()
	require.NoError(t, p.PrintList(List{Kind: "w"}, nil))
	require.Equal(t, "[]\n", out.String())
}

func TestPrintList_Name(t *testing.T) {
	var out, errOut bytes.Buffer
	p := testPrinterErr(FormatName, &out, &errOut)
	l := List{Kind: "w", Items: items("prod", "staging")}
	l.Name = nameOf(l)
	require.NoError(t, p.PrintList(l, nil))
	require.Equal(t, "prod\nstaging\n", out.String())

	out.Reset()
	require.NoError(t, p.PrintList(List{Kind: "w"}, nil))
	require.Empty(t, out.String(), "empty list prints nothing under -o name")
	require.Empty(t, errOut.String())
}

func TestPrintList_NextPageTokenOnStderrInEveryFormat(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatJSON, FormatYAML, FormatName} {
		var out, errOut bytes.Buffer
		p := testPrinterErr(f, &out, &errOut)
		l := List{Kind: "w", Items: items("a"), NextPageToken: "tok123"}
		l.Name = nameOf(l)
		require.NoError(t, p.PrintList(l, func(w *tabwriter.Writer) { Writeln(w, "a") }))
		require.Equal(t, "next page token: tok123\n", errOut.String(), "format %s", f)
		require.NotContains(t, out.String(), "tok123", "format %s", f)
	}
}

func TestPrintList_UnsupportedFormat(t *testing.T) {
	var out bytes.Buffer
	p := testPrinter("xml", &out)
	require.Error(t, p.PrintList(List{Kind: "w"}, nil))
}

func TestPrintOne(t *testing.T) {
	msg := structpb.NewStringValue("shop")

	var out bytes.Buffer
	require.NoError(t, testPrinter(FormatJSON, &out).PrintOne(msg, "shop", nil))
	require.Equal(t, "\"shop\"\n", out.String())

	out.Reset()
	require.NoError(t, testPrinter(FormatName, &out).PrintOne(msg, "shop", nil))
	require.Equal(t, "shop\n", out.String())

	out.Reset()
	require.NoError(t, testPrinter(FormatTable, &out).PrintOne(msg, "shop", func(w *tabwriter.Writer) {
		Writeln(w, "NAME\tAGE")
		Writeln(w, "shop\t1s")
	}))
	require.Equal(t, "NAME   AGE\nshop   1s\n", out.String())
}

func TestPrintResource_RejectsName(t *testing.T) {
	var out bytes.Buffer
	err := testPrinter(FormatName, &out).PrintResource(structpb.NewStringValue("x"), nil)
	require.ErrorContains(t, err, "-o name is not supported")
}

func TestMessages(t *testing.T) {
	typed := []*structpb.Value{structpb.NewStringValue("a"), structpb.NewStringValue("b")}
	got := Messages(typed)
	require.Len(t, got, 2)
	require.Same(t, typed[1], got[1])
}

func TestConfirmed(t *testing.T) {
	var errOut bytes.Buffer
	Confirmed(&errOut, "environment", "staging", "deleted")
	require.Equal(t, "environment \"staging\" deleted\n", errOut.String())
}

func TestParseFormat_Name(t *testing.T) {
	f, err := ParseFormat("name")
	require.NoError(t, err)
	require.Equal(t, FormatName, f)
	require.True(t, FormatWide.IsTable())
	require.True(t, FormatYAML.IsMachine())
	require.False(t, FormatName.IsMachine())
}

func TestFormatEnum(t *testing.T) {
	require.Equal(t, "PartiallyFailed", FormatEnum(runv1.RunStatus_RUN_STATUS_PARTIALLY_FAILED))
	require.Equal(t, "Succeeded", FormatEnum(runv1.RunStatus_RUN_STATUS_SUCCEEDED))
	require.Equal(t, None, FormatEnum(runv1.RunStatus_RUN_STATUS_UNSPECIFIED))
	require.Equal(t, "destroy-plan", FormatEnumKebab(agentv1.JobType_JOB_TYPE_DESTROY_PLAN))
	require.Equal(t, "ssh-key", FormatEnumKebab(credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY))
	require.Equal(t, None, FormatEnumKebab(credentialv1.CredentialType_CREDENTIAL_TYPE_UNSPECIFIED))
}
