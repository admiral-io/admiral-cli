package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

// Writef writes formatted output to w, swallowing the return values.
func Writef(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// Writeln writes a line to w, swallowing the return values.
func Writeln(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

// Printer handles output formatting for CLI commands.
type Printer struct {
	Format Format
	Out    io.Writer
}

// NewPrinter creates a new Printer with the given format.
func NewPrinter(format Format) *Printer {
	return &Printer{
		Format: format,
		Out:    os.Stdout,
	}
}

// PrintResource routes output based on the configured format.
// For table/wide formats, it calls the provided tableFn.
// For json/yaml, it marshals the proto message.
func (p *Printer) PrintResource(msg proto.Message, tableFn func(w *tabwriter.Writer)) error {
	switch p.Format {
	case FormatJSON:
		return p.printProtoJSON(msg)
	case FormatYAML:
		return p.printProtoYAML(msg)
	case FormatTable, FormatWide:
		return p.printTable(tableFn)
	default:
		return fmt.Errorf("unsupported format: %s", p.Format)
	}
}

// Detail represents a single key-value field in describe output.
type Detail struct {
	Key   string
	Value string
}

// Section represents a labeled group of details in describe output.
type Section struct {
	Name    string
	Details []Detail
}

// PrintDetail renders a single resource in kubectl-describe style.
// For json/yaml, it marshals the proto message.
// For table/wide, it renders key-value pairs grouped by section.
//
// Example output:
//
//	Name:         prod-us-east-1
//	ID:           a1b2c3d4-...
//	Health:       Healthy
//	Age:          14d
//
//	Labels:
//	  env=production
//	  team=platform
func (p *Printer) PrintDetail(msg proto.Message, sections []Section) error {
	switch p.Format {
	case FormatJSON:
		return p.printProtoJSON(msg)
	case FormatYAML:
		return p.printProtoYAML(msg)
	case FormatTable, FormatWide:
		return p.printDetail(sections)
	default:
		return fmt.Errorf("unsupported format: %s", p.Format)
	}
}

func (p *Printer) printDetail(sections []Section) error {
	w := tabwriter.NewWriter(p.Out, 0, 0, 3, ' ', 0)

	for i, section := range sections {
		if i > 0 {
			Writeln(w)
		}
		if section.Name != "" {
			Writef(w, "%s:\n", section.Name)
		}

		prefix := ""
		if section.Name != "" {
			prefix = "  "
		}

		for _, d := range section.Details {
			Writef(w, "%s%s:\t%s\n", prefix, d.Key, d.Value)
		}
	}

	return w.Flush()
}

// PrintToken prints a one-time token with a warning that it won't be shown again.
func PrintToken(stderr io.Writer, token string) {
	Writeln(stderr)
	Writeln(stderr, "WARNING: Save this token — it will not be shown again.")
	Writef(stderr, "Token: %s\n", token)
}

func (p *Printer) printProtoJSON(msg proto.Message) error {
	opts := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}
	b, err := opts.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	_, err = fmt.Fprintln(p.Out, string(b))
	return err
}

func (p *Printer) printProtoYAML(msg proto.Message) error {
	opts := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonBytes, err := opts.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	var obj any
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return fmt.Errorf("failed to parse json: %w", err)
	}

	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml: %w", err)
	}

	_, err = p.Out.Write(yamlBytes)
	return err
}

func (p *Printer) printTable(fn func(w *tabwriter.Writer)) error {
	w := tabwriter.NewWriter(p.Out, 0, 0, 3, ' ', 0)
	fn(w)
	return w.Flush()
}

// FormatScopes formats a slice of scopes for display.
func FormatScopes(scopes []string) string {
	if len(scopes) == 0 {
		return "<none>"
	}
	return strings.Join(scopes, ", ")
}
