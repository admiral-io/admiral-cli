package output

import (
	"bytes"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/iostreams"
)

type widget struct {
	name, desc, id string
}

var widgetTable = Table[widget]{
	{Header: "NAME", Cell: func(w widget) string { return w.name }},
	{Header: "DESCRIPTION", Truncate: 10, Cell: func(w widget) string { return w.desc }},
	{Header: "created by", Wide: true, Cell: func(w widget) string { return w.id }},
}

func renderTable(t *testing.T, p *Printer, rows ...widget) string {
	t.Helper()
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)
	widgetTable.Render(p, rows...)(w)
	require.NoError(t, w.Flush())
	return buf.String()
}

func ttyPrinter(format Format, out *bytes.Buffer) *Printer {
	return &Printer{
		Format: format,
		IO:     iostreams.New(strings.NewReader(""), out, &bytes.Buffer{}, func(k string) string { return map[string]string{"ADMIRAL_FORCE_TTY": "100"}[k] }),
	}
}

func TestTable_NarrowHidesWideColumns(t *testing.T) {
	var out bytes.Buffer
	got := renderTable(t, testPrinter(FormatTable, &out), widget{"a", "short", "u1"})
	require.Equal(t, "NAME   DESCRIPTION\na      short\n", got)
}

func TestTable_WideAppendsAndHyphenatesHeaders(t *testing.T) {
	var out bytes.Buffer
	got := renderTable(t, testPrinter(FormatWide, &out), widget{"a", "short", "u1"})
	require.Equal(t, "NAME   DESCRIPTION   CREATED-BY\na      short         u1\n", got)
}

func TestTable_EmptyCellIsNone(t *testing.T) {
	var out bytes.Buffer
	got := renderTable(t, testPrinter(FormatTable, &out), widget{"a", "", ""})
	require.Equal(t, "NAME   DESCRIPTION\na      <none>\n", got)
}

func TestTable_TruncatesOnlyOnTTY(t *testing.T) {
	long := widget{"a", "a description that is long", "u1"}

	var out bytes.Buffer
	piped := renderTable(t, testPrinter(FormatTable, &out), long)
	require.Contains(t, piped, "a description that is long", "piped output is never truncated")

	tty := renderTable(t, ttyPrinter(FormatTable, &out), long)
	require.Contains(t, tty, "a descrip…")
	require.NotContains(t, tty, "is long")
}

func TestTable_HeaderOnlyWhenNoRows(t *testing.T) {
	var out bytes.Buffer
	got := renderTable(t, testPrinter(FormatTable, &out))
	require.Equal(t, "NAME   DESCRIPTION\n", got, "PrintList guards the empty case; Render itself just prints the header")
}

func TestTable_Write(t *testing.T) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)
	widgetTable.Write(w, testPrinter(FormatTable, &bytes.Buffer{}), widget{"a", "x", ""}, widget{"b", "y", ""})
	require.NoError(t, w.Flush())
	require.Equal(t, "NAME   DESCRIPTION\na      x\nb      y\n", buf.String())
}

func TestHeaderName(t *testing.T) {
	require.Equal(t, "CHANGE-SET", headerName("CHANGE SET"))
	require.Equal(t, "CREATED-BY", headerName(" created by "))
	require.Equal(t, "AGE", headerName("AGE"))
}
