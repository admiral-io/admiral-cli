package output

import (
	"strings"
	"text/tabwriter"
)

// Column describes one column of a resource table. A resource declares its
// columns once, in cmd/<pkg>/output.go, and list, get and the create/update
// echo all render from that one declaration, so narrow and wide output can
// never drift apart.
type Column[T any] struct {
	// Header is shown in UPPER-HYPHEN form; spaces are converted to hyphens
	// so `awk` column counting works ("CHANGE SET" renders as CHANGE-SET).
	Header string
	// Cell renders the value. An empty result prints as <none>.
	Cell func(T) string
	// Wide marks a column that is only rendered under -o wide.
	Wide bool
	// Truncate, when non-zero, caps the cell at that many characters with an
	// ellipsis — but only when stdout is a terminal. Piped output is never
	// truncated. Use it for free text (descriptions, titles), never for
	// names or IDs.
	Truncate int
}

// Table is an ordered list of columns for one resource type.
type Table[T any] []Column[T]

// Render returns a table renderer for PrintList or PrintOne that prints the
// header row and one row per item, honoring the printer's format (-o wide)
// and whether stdout is a terminal (truncation).
func (t Table[T]) Render(p *Printer, rows ...T) func(w *tabwriter.Writer) {
	wide := p.Format == FormatWide
	tty := p.IO.IsStdoutTTY()
	return func(w *tabwriter.Writer) {
		t.write(w, wide, tty, rows)
	}
}

// Write renders the table straight to w. It is for composite views that
// place several tables in one output; everything else goes through Render.
func (t Table[T]) Write(w *tabwriter.Writer, p *Printer, rows ...T) {
	t.write(w, p.Format == FormatWide, p.IO.IsStdoutTTY(), rows)
}

func (t Table[T]) write(w *tabwriter.Writer, wide, tty bool, rows []T) {
	cols := t.visible(wide)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = headerName(c.Header)
	}
	Writeln(w, strings.Join(headers, "\t"))

	cells := make([]string, len(cols))
	for _, r := range rows {
		for i, c := range cols {
			v := c.Cell(r)
			if strings.TrimSpace(v) == "" {
				v = None
			} else if tty && c.Truncate > 0 {
				v = Truncate(v, c.Truncate)
			}
			cells[i] = v
		}
		Writeln(w, strings.Join(cells, "\t"))
	}
}

func (t Table[T]) visible(wide bool) []Column[T] {
	out := make([]Column[T], 0, len(t))
	for _, c := range t {
		if c.Wide && !wide {
			continue
		}
		out = append(out, c)
	}
	return out
}

func headerName(h string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(h), " ", "-"))
}
