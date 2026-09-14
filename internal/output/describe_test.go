package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

func TestDescribe_Layout(t *testing.T) {
	d := NewDescribe()
	d.Field("Name", "prod")
	d.Field("Application", "shop")
	d.Field("Description", "")
	d.Fields("Labels", []string{"tier=1", "team=commerce"})
	d.Section("Components", func(b *Block) {
		b.Block("api", func(b *Block) {
			b.Field("Kind", "Workload")
			b.Field("Message", "")
		})
	})
	d.Table("Events", []string{"Age", "Type", "Message"}, [][]string{{"3h", "Normal", "run succeeded"}, {"2d", "Warning", ""}})
	d.Hint("To see the transcript, run: admiral run logs run-1")

	var buf bytes.Buffer
	require.NoError(t, d.Render(&buf))
	want := `Name:         prod
Application:  shop
Description:  <none>
Labels:       tier=1
              team=commerce

Components:
  api:
    Kind:     Workload
    Message:  <none>

Events:
  Age  Type     Message
  ---  ----     -------
  3h   Normal   run succeeded
  2d   Warning  <none>

To see the transcript, run: admiral run logs run-1
`
	require.Equal(t, want, buf.String())
}

func TestDescribe_EmptyFieldsIsNone(t *testing.T) {
	d := NewDescribe()
	d.Fields("Labels", nil)
	var buf bytes.Buffer
	require.NoError(t, d.Render(&buf))
	require.Equal(t, "Labels:  <none>\n", buf.String())
}

func TestDescribe_UnavailableSection(t *testing.T) {
	d := NewDescribe()
	d.Field("Name", "shop")
	d.Unavailable("Environments", "permission denied: missing required scope env:read")
	d.Table("Recent Runs", []string{"ID", "Status"}, [][]string{{"run-1", "Succeeded"}})
	d.Hint(cmderr.ScopeHint)

	var buf bytes.Buffer
	require.NoError(t, d.Render(&buf))
	want := `Name:  shop

Environments:
  <unavailable: permission denied: missing required scope env:read>

Recent Runs:
  ID     Status
  --     ------
  run-1  Succeeded

Run 'admiral auth status' to see the active credential and its scopes.
`
	require.Equal(t, want, buf.String())
}

func TestPrintDescribe_RejectsMachineFormats(t *testing.T) {
	var out bytes.Buffer
	err := testPrinter(FormatJSON, &out).PrintDescribe(NewDescribe(), "admiral env get prod --app shop")
	require.EqualError(t, err, "describe has no -o json output")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Equal(t, "Run 'admiral env get prod --app shop -o json' for machine-readable output.", cmderr.Hint(err))

	d := NewDescribe()
	d.Field("Name", "x")
	require.NoError(t, testPrinter(FormatWide, &out).PrintDescribe(d, ""))
	require.Equal(t, "Name:  x\n", out.String())
}
