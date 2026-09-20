package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

	"go.admiral.io/cli/internal/iostreams"
)

// Printer renders command results in the format the user asked for.
// Everything that is "the answer" goes to IO.Out; everything else (empty
// results, next-page tokens, confirmations) goes to IO.Err, so stdout stays
// clean for pipelines.
type Printer struct {
	Format Format
	IO     *iostreams.Streams
}

// NewPrinter creates a Printer bound to cmd's streams.
func NewPrinter(cmd *cobra.Command, format Format) *Printer {
	return &Printer{Format: format, IO: iostreams.FromCommand(cmd)}
}

// Out is the writer for the command's answer.
func (p *Printer) Out() io.Writer { return p.IO.Out }

// Err is the writer for messaging.
func (p *Printer) Err() io.Writer { return p.IO.Err }

// List describes a collection result for PrintList.
type List struct {
	// Kind is the plural noun used in the empty message: "environments".
	Kind string
	// Scope, when set, is appended to the empty message: "No runs found in shop/prod."
	Scope string
	// Items are the resources, in display order. Use Messages to build it
	// from a typed slice.
	Items []proto.Message
	// Name returns the identifier printed by -o name for Items[i].
	Name func(i int) string
	// NextPageToken, when set, is reported on stderr in every format.
	NextPageToken string
}

// Messages converts a typed proto slice into []proto.Message for List.Items.
func Messages[T proto.Message](items []T) []proto.Message {
	out := make([]proto.Message, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

// PrintList renders a list result.
//
//   - table/wide: tableFn renders header and rows; an empty list prints
//     "No <kind> found." to stderr and nothing to stdout.
//   - json/yaml: a bare array of the full API objects; "[]" when empty.
//   - name: one identifier per line; nothing when empty.
//
// When NextPageToken is set it is printed to stderr after the result in
// every format, so JSON stays a plain array.
func (p *Printer) PrintList(l List, tableFn func(w *tabwriter.Writer)) error {
	var err error
	switch p.Format {
	case FormatJSON:
		err = p.printJSONArray(l.Items)
	case FormatYAML:
		err = p.printYAMLArray(l.Items)
	case FormatName:
		if l.Name == nil && len(l.Items) > 0 {
			return errors.New("-o name is not supported by this command")
		}
		for i := range l.Items {
			Writeln(p.IO.Out, l.Name(i))
		}
	case FormatTable, FormatWide:
		if len(l.Items) == 0 {
			PrintEmpty(p.IO.Err, l.Kind, l.Scope)
			return nil
		}
		err = p.printTable(tableFn)
	default:
		return fmt.Errorf("unsupported format: %s", p.Format)
	}
	if err != nil {
		return err
	}
	if l.NextPageToken != "" {
		Writef(p.IO.Err, "next page token: %s (pass it with --page-token, or use --all)\n", l.NextPageToken)
	}
	return nil
}

// PrintOne renders a single resource: the full object for json/yaml, name
// for -o name, and tableFn for table/wide.
func (p *Printer) PrintOne(msg proto.Message, name string, tableFn func(w *tabwriter.Writer)) error {
	switch p.Format {
	case FormatJSON:
		return p.printJSON(msg)
	case FormatYAML:
		return p.printYAML(msg)
	case FormatName:
		Writeln(p.IO.Out, name)
		return nil
	case FormatTable, FormatWide:
		return p.printTable(tableFn)
	default:
		return fmt.Errorf("unsupported format: %s", p.Format)
	}
}

// PrintResource renders a message that is not a single named resource
// (composite views such as a run with its revisions). It behaves like
// PrintOne but rejects -o name, which has no meaning for it.
func (p *Printer) PrintResource(msg proto.Message, tableFn func(w *tabwriter.Writer)) error {
	if p.Format == FormatName {
		return fmt.Errorf("-o name is not supported by this command")
	}
	return p.PrintOne(msg, "", tableFn)
}

// marshalJSON renders msg through protojson and then re-encodes it with
// encoding/json. protojson deliberately randomizes its whitespace so that
// nobody depends on byte-for-byte output; re-encoding gives a stable form,
// indented when stdout is a terminal and compact when piped.
func (p *Printer) marshalJSON(msg proto.Message) ([]byte, error) {
	raw, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}
	return p.normalizeJSON(raw)
}

func (p *Printer) normalizeJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if p.IO.IsStdoutTTY() {
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
			return nil, fmt.Errorf("failed to format response: %w", err)
		}
	} else {
		if err := json.Compact(&buf, raw); err != nil {
			return nil, fmt.Errorf("failed to format response: %w", err)
		}
	}
	return buf.Bytes(), nil
}

func (p *Printer) printJSON(msg proto.Message) error {
	b, err := p.marshalJSON(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(p.IO.Out, string(b))
	return err
}

func (p *Printer) printJSONArray(items []proto.Message) error {
	parts := make([]string, len(items))
	for i, it := range items {
		raw, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(it)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		parts[i] = string(raw)
	}
	b, err := p.normalizeJSON([]byte("[" + strings.Join(parts, ",") + "]"))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(p.IO.Out, string(b))
	return err
}

func (p *Printer) printYAML(msg proto.Message) error {
	obj, err := toYAMLValue(msg)
	if err != nil {
		return err
	}
	return p.writeYAML(obj)
}

func (p *Printer) printYAMLArray(items []proto.Message) error {
	objs := make([]any, len(items))
	for i, it := range items {
		obj, err := toYAMLValue(it)
		if err != nil {
			return err
		}
		objs[i] = obj
	}
	return p.writeYAML(objs)
}

func (p *Printer) writeYAML(obj any) error {
	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml: %w", err)
	}
	_, err = p.IO.Out.Write(yamlBytes)
	return err
}

func (p *Printer) printTable(fn func(w *tabwriter.Writer)) error {
	w := tabwriter.NewWriter(p.IO.Out, 0, 0, 3, ' ', 0)
	fn(w)
	return w.Flush()
}

// PrintEmpty writes the kubectl-style empty-result message to stderr:
// "No <kind> found." or "No <kind> found in <scope>." when scope is set.
func PrintEmpty(stderr io.Writer, kind, scope string) {
	if scope != "" {
		Writef(stderr, "No %s found in %s.\n", kind, scope)
		return
	}
	Writef(stderr, "No %s found.\n", kind)
}

// Confirmed writes the one-line confirmation for a mutation that returns no
// resource, in kubectl's shape: `environment "staging" deleted`. It goes to
// stderr so stdout stays empty for $(...) and pipelines.
func Confirmed(stderr io.Writer, kind, name, verbed string) {
	Writef(stderr, "%s %q %s\n", kind, name, verbed)
}

// Writef writes formatted output to w, swallowing the return values.
func Writef(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// Writeln writes a line to w, swallowing the return values.
func Writeln(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

// toYAMLValue goes through protojson so YAML field names and enum values
// match the JSON output exactly.
func toYAMLValue(msg proto.Message) (any, error) {
	jsonBytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}
	var obj any
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}
	return obj, nil
}
