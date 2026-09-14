package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/iostreams"
)

// ---------------------------------------------------------------------------
// format.go
// ---------------------------------------------------------------------------

func TestParseFormat(t *testing.T) {
	valid := []struct {
		input string
		want  Format
	}{
		{"table", FormatTable},
		{"json", FormatJSON},
		{"yaml", FormatYAML},
		{"wide", FormatWide},
	}
	for _, tc := range valid {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseFormat(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := ParseFormat("xml")
		if err == nil {
			t.Fatal("expected error for invalid format")
		}
		if !strings.Contains(err.Error(), "invalid output format") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, err := ParseFormat("")
		if err == nil {
			t.Fatal("expected error for empty format")
		}
	})
}

func TestFormat_String(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatTable, "table"},
		{FormatJSON, "json"},
		{FormatYAML, "yaml"},
		{FormatWide, "wide"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.format.String(); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestWritef(t *testing.T) {
	var buf bytes.Buffer
	Writef(&buf, "hello %s %d", "world", 42)
	if got := buf.String(); got != "hello world 42" {
		t.Fatalf("want %q, got %q", "hello world 42", got)
	}
}

func TestWriteln(t *testing.T) {
	var buf bytes.Buffer
	Writeln(&buf, "hello", "world")
	if got := buf.String(); got != "hello world\n" {
		t.Fatalf("want %q, got %q", "hello world\n", got)
	}
}

func TestWriteln_Empty(t *testing.T) {
	var buf bytes.Buffer
	Writeln(&buf)
	if got := buf.String(); got != "\n" {
		t.Fatalf("want %q, got %q", "\n", got)
	}
}

// ---------------------------------------------------------------------------
// printer.go
// ---------------------------------------------------------------------------

// testPrinter builds a Printer whose stdout is buf and whose stderr is a
// separate buffer, both non-TTY, with a pinned empty environment.
func testPrinter(format Format, out *bytes.Buffer) *Printer {
	return &Printer{
		Format: format,
		IO:     iostreams.New(strings.NewReader(""), out, &bytes.Buffer{}, func(string) string { return "" }),
	}
}

func testPrinterErr(format Format, out, errOut *bytes.Buffer) *Printer {
	return &Printer{
		Format: format,
		IO:     iostreams.New(strings.NewReader(""), out, errOut, func(string) string { return "" }),
	}
}

func TestNewPrinter(t *testing.T) {
	p := NewPrinter(&cobra.Command{}, FormatJSON)
	if p.Format != FormatJSON {
		t.Fatalf("want format %q, got %q", FormatJSON, p.Format)
	}
	if p.Out() == nil {
		t.Fatal("expected non-nil writer")
	}
}

func TestPrintResource_Table(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatTable, &buf)

	err := p.PrintResource(structpb.NewStringValue("test"), func(w *tabwriter.Writer) {
		fmt.Fprintln(w, "NAME\tAGE")
		fmt.Fprintln(w, "foo\t5d")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "foo") {
		t.Fatalf("unexpected table output: %q", out)
	}
}

func TestPrintResource_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatJSON, &buf)

	msg := structpb.NewStringValue("hello")
	err := p.PrintResource(msg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected 'hello' in JSON output: %q", buf.String())
	}
}

func TestPrintResource_YAML(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatYAML, &buf)

	msg := structpb.NewStringValue("world")
	err := p.PrintResource(msg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "world") {
		t.Fatalf("expected 'world' in YAML output: %q", buf.String())
	}
}

func TestPrintResource_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter("xml", &buf)

	err := p.PrintResource(structpb.NewStringValue("test"), nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// table.go
// ---------------------------------------------------------------------------

func TestFormatAge(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := FormatAge(nil); got != None {
			t.Fatalf("want %q, got %q", None, got)
		}
	})
	t.Run("seconds", func(t *testing.T) {
		ts := timestamppb.New(time.Now().Add(-30 * time.Second))
		got := FormatAge(ts)
		if !strings.HasSuffix(got, "s") {
			t.Fatalf("expected seconds suffix, got %q", got)
		}
	})
	t.Run("minutes", func(t *testing.T) {
		ts := timestamppb.New(time.Now().Add(-5 * time.Minute))
		got := FormatAge(ts)
		if !strings.HasSuffix(got, "m") {
			t.Fatalf("expected minutes suffix, got %q", got)
		}
	})
	t.Run("hours", func(t *testing.T) {
		ts := timestamppb.New(time.Now().Add(-3 * time.Hour))
		got := FormatAge(ts)
		if !strings.HasSuffix(got, "h") {
			t.Fatalf("expected hours suffix, got %q", got)
		}
	})
	t.Run("days", func(t *testing.T) {
		ts := timestamppb.New(time.Now().Add(-72 * time.Hour))
		got := FormatAge(ts)
		if !strings.HasSuffix(got, "d") {
			t.Fatalf("expected days suffix, got %q", got)
		}
	})
}

func TestFormatTimestamp(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := FormatTimestamp(nil); got != None {
			t.Fatalf("want %q, got %q", None, got)
		}
	})
	t.Run("valid", func(t *testing.T) {
		now := time.Now().UTC()
		ts := timestamppb.New(now)
		got := FormatTimestamp(ts)
		want := now.Format(time.RFC3339)
		if got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	})
}

func TestFormatLabels(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := FormatLabels(nil); got != None {
			t.Fatalf("want %q, got %q", None, got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got := FormatLabels(map[string]string{}); got != None {
			t.Fatalf("want %q, got %q", None, got)
		}
	})
	t.Run("single", func(t *testing.T) {
		got := FormatLabels(map[string]string{"env": "prod"})
		if got != "env=prod" {
			t.Fatalf("want %q, got %q", "env=prod", got)
		}
	})
	t.Run("multiple are sorted by key", func(t *testing.T) {
		got := FormatLabels(map[string]string{"team": "core", "env": "prod", "app": "web"})
		if want := "app=web,env=prod,team=core"; got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	})
}

func TestHumanDuration(t *testing.T) {
	// Values from kubectl's duration_test.go.
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-2 * time.Second, "<invalid>"},
		{-1 * time.Second, "0s"},
		{0, "0s"},
		{30 * time.Second, "30s"},
		{119 * time.Second, "119s"},
		{2 * time.Minute, "2m"},
		{5*time.Minute + 12*time.Second, "5m12s"},
		{10 * time.Minute, "10m"},
		{10*time.Minute + 30*time.Second, "10m"},
		{2*time.Hour + 59*time.Minute, "179m"},
		{3 * time.Hour, "3h"},
		{3*time.Hour + 4*time.Minute, "3h4m"},
		{8 * time.Hour, "8h"},
		{47 * time.Hour, "47h"},
		{48 * time.Hour, "2d"},
		{5*24*time.Hour + 3*time.Hour, "5d3h"},
		{8 * 24 * time.Hour, "8d"},
		{9*24*time.Hour + 3*time.Hour, "9d"},
		{365 * 24 * time.Hour, "365d"},
		{2 * 365 * 24 * time.Hour, "2y"},
		{2*365*24*time.Hour + 24*time.Hour, "2y1d"},
		{8 * 365 * 24 * time.Hour, "8y"},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, HumanDuration(tc.d), "%v", tc.d)
	}
}

// ---------------------------------------------------------------------------
// Additional printer.go tests
// ---------------------------------------------------------------------------

func TestNewPrinter_DefaultsToStdout(t *testing.T) {
	p := NewPrinter(&cobra.Command{}, FormatTable)
	require.Equal(t, FormatTable, p.Format)
	require.Equal(t, os.Stdout, p.Out())
}

func TestPrintResource_Wide(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatWide, &buf)

	err := p.PrintResource(structpb.NewStringValue("test"), func(w *tabwriter.Writer) {
		fmt.Fprintln(w, "NAME\tAGE\tLABELS")
		fmt.Fprintln(w, "foo\t5d\tenv=prod")
	})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "NAME")
	require.Contains(t, out, "LABELS")
	require.Contains(t, out, "env=prod")
}

func TestPrintResource_JSON_ValidStructure(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatJSON, &buf)

	val, err := structpb.NewStruct(map[string]any{
		"name": "test-cluster",
		"id":   "abc-123",
	})
	require.NoError(t, err)

	err = p.PrintResource(val, nil)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Equal(t, "test-cluster", parsed["name"])
	require.Equal(t, "abc-123", parsed["id"])
}

func TestPrintResource_JSON_Multiline(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatJSON, &buf)

	err := p.PrintResource(structpb.NewStringValue("test"), nil)
	require.NoError(t, err)

	// Piped stdout is compact: exactly one line.
	require.Equal(t, 1, strings.Count(buf.String(), "\n"))
}

func TestPrintResource_YAML_Structure(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatYAML, &buf)

	val, err := structpb.NewStruct(map[string]any{
		"name": "test-runner",
	})
	require.NoError(t, err)

	err = p.PrintResource(val, nil)
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "name:")
	require.Contains(t, out, "test-runner")
}

func TestPrintResource_NilMessage_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatJSON, &buf)

	// nil proto.Message should marshal as empty JSON object
	err := p.PrintResource(nil, nil)
	require.NoError(t, err)
	require.True(t, json.Valid(buf.Bytes()))
}

func TestPrintResource_NilMessage_YAML(t *testing.T) {
	var buf bytes.Buffer
	p := testPrinter(FormatYAML, &buf)

	err := p.PrintResource(nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, buf.String())
}

func TestWritef_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	Writef(&buf, "literal string")
	require.Equal(t, "literal string", buf.String())
}

func TestWriteln_SingleArg(t *testing.T) {
	var buf bytes.Buffer
	Writeln(&buf, "single")
	require.Equal(t, "single\n", buf.String())
}

// ---------------------------------------------------------------------------
// Additional format.go tests
// ---------------------------------------------------------------------------

func TestParseFormat_CaseSensitive(t *testing.T) {
	// Formats must be lowercase
	for _, input := range []string{"TABLE", "Table", "JSON", "Json", "YAML", "WIDE"} {
		_, err := ParseFormat(input)
		require.Error(t, err, "expected error for %q", input)
	}
}

func TestFormat_String_RoundTrip(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatJSON, FormatYAML, FormatWide} {
		parsed, err := ParseFormat(f.String())
		require.NoError(t, err)
		require.Equal(t, f, parsed)
	}
}

// ---------------------------------------------------------------------------
// Additional table.go tests
// ---------------------------------------------------------------------------

func TestFormatAge_RecentTimestamp(t *testing.T) {
	// A timestamp from 1 second ago should show as "1s"
	ts := timestamppb.New(time.Now().Add(-1 * time.Second))
	got := FormatAge(ts)
	require.Regexp(t, `^\d+s$`, got)
}

func TestFormatAge_ExactBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		age    time.Duration
		suffix string
	}{
		{"119 seconds", 119 * time.Second, "s"},
		{"120 seconds", 120 * time.Second, "m"},
		{"179 minutes", 179 * time.Minute, "m"},
		{"180 minutes", 180 * time.Minute, "h"},
		{"47 hours", 47 * time.Hour, "h"},
		{"48 hours", 48 * time.Hour, "d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := timestamppb.New(time.Now().Add(-tc.age))
			got := FormatAge(ts)
			require.True(t, strings.HasSuffix(got, tc.suffix),
				"age %v: want suffix %q, got %q", tc.age, tc.suffix, got)
		})
	}
}

func TestFormatTimestamp_UTC(t *testing.T) {
	// Ensure output is in UTC RFC3339 format
	fixedTime := time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)
	ts := timestamppb.New(fixedTime)
	got := FormatTimestamp(ts)
	require.Equal(t, "2025-06-15T12:30:00Z", got)
}

func TestFormatLabels_Sorted(t *testing.T) {
	// With a single label, output should be deterministic
	got := FormatLabels(map[string]string{"team": "platform"})
	require.Equal(t, "team=platform", got)
}

func TestFormatLabels_SpecialCharacters(t *testing.T) {
	got := FormatLabels(map[string]string{"app.kubernetes.io/name": "admiral"})
	require.Equal(t, "app.kubernetes.io/name=admiral", got)
}

func TestTruncate_SmallWidths(t *testing.T) {
	require.Equal(t, "abcdefgh", Truncate("abcdefgh", 8))
	require.Equal(t, "abcdef…", Truncate("abcdefgh", 7))
	require.Equal(t, "ab…", Truncate("abcdefgh", 3))
	require.Equal(t, "a", Truncate("abcdefgh", 1), "no room for an ellipsis: plain cut")
	require.Equal(t, "héll…", Truncate("héllo wörld", 5), "counts runes, not bytes")
	require.Equal(t, "", Truncate("abcdefgh", 0))
	require.Equal(t, "", Truncate("abcdefgh", -5), "negative width does not panic")
}
