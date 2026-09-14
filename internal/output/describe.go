package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"google.golang.org/protobuf/proto"

	"go.admiral.io/cli/internal/cmderr"
)

// Describe builds a kubectl-style detail view: aligned `Key:  value` fields,
// named sections with nested blocks, embedded tables with Title Case
// headers and a dashes underline, and a trailing next-step hint.
//
//	d := output.NewDescribe()
//	d.Field("Name", e.Name)
//	d.Fields("Labels", labels)
//	d.Section("Components", func(b *output.Block) {
//		b.Block("api", func(b *output.Block) { b.Field("Kind", "Workload") })
//	})
//	d.Table("Events", []string{"Age", "Type", "Message"}, rows)
//	d.Hint("To see the transcript, run: admiral run logs " + id)
type Describe struct {
	root  Block
	hints []string
}

// Block is a group of fields, nested blocks and tables at one indent level.
type Block struct {
	items []item
}

type item struct {
	key    string
	values []string // field; nil for non-fields
	block  *Block   // nested block
	table  *table   // embedded table
	line   string   // bare text line, no key
}

type table struct {
	headers []string
	rows    [][]string
}

// NewDescribe returns an empty detail view.
func NewDescribe() *Describe { return &Describe{} }

// Field adds a top-level `Key:  value` line. An empty value prints <none>.
func (d *Describe) Field(key, value string) { d.root.Field(key, value) }

// Fields adds a top-level key whose values continue on indented lines.
func (d *Describe) Fields(key string, values []string) { d.root.Fields(key, values) }

// Section adds a named section whose content is built by fn, indented two
// spaces under a `Name:` header.
func (d *Describe) Section(name string, fn func(b *Block)) { d.root.Block(name, fn) }

// Table adds a named section containing one table.
func (d *Describe) Table(name string, headers []string, rows [][]string) {
	d.root.Block(name, func(b *Block) { b.Table(headers, rows) })
}

// Unavailable adds a named section whose content could not be loaded,
// rendered as `<unavailable: reason>` in place of the table or block.
// describe is read-only and assembles several reads; one failing read (a
// missing scope, an endpoint the server lacks) should cost that section,
// not the whole view.
func (d *Describe) Unavailable(name, reason string) {
	d.root.Block(name, func(b *Block) { b.Line("<unavailable: " + reason + ">") })
}

// Hint adds a trailing next-step line, printed after a blank line.
func (d *Describe) Hint(line string) { d.hints = append(d.hints, line) }

// Render writes the view to w.
func (d *Describe) Render(w io.Writer) error {
	if err := d.root.render(w, 0); err != nil {
		return err
	}
	for _, h := range d.hints {
		if _, err := fmt.Fprintf(w, "\n%s\n", h); err != nil {
			return err
		}
	}
	return nil
}

// Field adds a `Key:  value` line to the block.
func (b *Block) Field(key, value string) {
	if strings.TrimSpace(value) == "" {
		value = None
	}
	b.items = append(b.items, item{key: key, values: []string{value}})
}

// Fields adds a key whose values continue on following lines, aligned
// under the first. An empty list prints <none>.
func (b *Block) Fields(key string, values []string) {
	if len(values) == 0 {
		values = []string{None}
	}
	b.items = append(b.items, item{key: key, values: values})
}

// Line adds a bare text line with no key, such as an unavailable note.
func (b *Block) Line(text string) {
	b.items = append(b.items, item{line: text})
}

// Block adds a nested block under a `name:` header.
func (b *Block) Block(name string, fn func(b *Block)) {
	nb := &Block{}
	fn(nb)
	b.items = append(b.items, item{key: name, block: nb})
}

// Table adds an embedded table. Headers are rendered as given (Title Case)
// with a dashes underline; empty cells print <none>.
func (b *Block) Table(headers []string, rows [][]string) {
	b.items = append(b.items, item{table: &table{headers: headers, rows: rows}})
}

// render writes the block at the given indent. Consecutive fields share one
// tabwriter so their values align; a nested block or table starts a new
// alignment group, and top-level sections are separated by a blank line.
func (b *Block) render(w io.Writer, indent int) error {
	pad := strings.Repeat(" ", indent)
	var tw *tabwriter.Writer
	flush := func() error {
		if tw == nil {
			return nil
		}
		err := tw.Flush()
		tw = nil
		return err
	}

	for i, it := range b.items {
		switch {
		case it.block != nil:
			if err := flush(); err != nil {
				return err
			}
			if indent == 0 && i > 0 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%s%s:\n", pad, it.key); err != nil {
				return err
			}
			if err := it.block.render(w, indent+2); err != nil {
				return err
			}
		case it.table != nil:
			if err := flush(); err != nil {
				return err
			}
			if err := it.table.render(w, indent); err != nil {
				return err
			}
		case it.line != "":
			if err := flush(); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s%s\n", pad, it.line); err != nil {
				return err
			}
		default:
			if tw == nil {
				tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			}
			Writef(tw, "%s%s:\t%s\n", pad, it.key, it.values[0])
			for _, v := range it.values[1:] {
				Writef(tw, "%s\t%s\n", pad, v)
			}
		}
	}
	return flush()
}

func (t *table) render(w io.Writer, indent int) error {
	pad := strings.Repeat(" ", indent)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	Writef(tw, "%s%s\n", pad, strings.Join(t.headers, "\t"))
	under := make([]string, len(t.headers))
	for i, h := range t.headers {
		under[i] = strings.Repeat("-", len(h))
	}
	Writef(tw, "%s%s\n", pad, strings.Join(under, "\t"))
	for _, r := range t.rows {
		cells := make([]string, len(t.headers))
		for i := range cells {
			if i < len(r) && strings.TrimSpace(r[i]) != "" {
				cells[i] = r[i]
			} else {
				cells[i] = None
			}
		}
		Writef(tw, "%s%s\n", pad, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

// PrintDescribe renders a Describe view. describe is a human view only: the
// machine path is `get -o json` plus the child list verbs, so any -o other
// than table is a usage error naming that path.
func (p *Printer) PrintDescribe(d *Describe, getCommand string) error {
	if !p.Format.IsTable() {
		return cmderr.UsageHint("Run '"+getCommand+" -o "+p.Format.String()+"' for machine-readable output.",
			"describe has no -o %s output", p.Format)
	}
	return d.Render(p.IO.Out)
}

// PrintStatus renders a status-style view: the Describe layout on a
// terminal, and v as JSON or YAML under -o json|yaml. v may be a proto
// message (rendered through protojson) or any Go value with json tags.
// Used by whoami, auth status, config list, agent status and similar
// commands whose answer is a document rather than a resource row.
func (p *Printer) PrintStatus(v any, d *Describe) error {
	switch p.Format {
	case FormatJSON:
		if msg, ok := v.(proto.Message); ok {
			return p.printJSON(msg)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		b, err := p.normalizeJSON(raw)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(p.IO.Out, string(b))
		return err
	case FormatYAML:
		if msg, ok := v.(proto.Message); ok {
			return p.printYAML(msg)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		var obj any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return fmt.Errorf("failed to parse json: %w", err)
		}
		return p.writeYAML(obj)
	case FormatTable, FormatWide:
		return d.Render(p.IO.Out)
	case FormatName:
		return fmt.Errorf("-o name is not supported by this command")
	default:
		return fmt.Errorf("unsupported format: %s", p.Format)
	}
}
