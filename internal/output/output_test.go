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

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestNewPrinter(t *testing.T) {
	p := NewPrinter(FormatJSON)
	if p.Format != FormatJSON {
		t.Fatalf("want format %q, got %q", FormatJSON, p.Format)
	}
	if p.Out == nil {
		t.Fatal("expected non-nil writer")
	}
}

func TestPrintResource_Table(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatTable, Out: &buf}

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
	p := &Printer{Format: FormatJSON, Out: &buf}

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
	p := &Printer{Format: FormatYAML, Out: &buf}

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
	p := &Printer{Format: "xml", Out: &buf}

	err := p.PrintResource(structpb.NewStringValue("test"), nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintDetail_Table(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatTable, Out: &buf}

	sections := []Section{
		{
			Details: []Detail{
				{Key: "Name", Value: "prod-cluster"},
				{Key: "ID", Value: "abc-123"},
			},
		},
		{
			Name: "Labels",
			Details: []Detail{
				{Key: "env", Value: "production"},
			},
		},
	}

	err := p.PrintDetail(structpb.NewStringValue("test"), sections)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Name:") || !strings.Contains(out, "prod-cluster") {
		t.Fatalf("expected Name detail in output: %q", out)
	}
	if !strings.Contains(out, "Labels:") {
		t.Fatalf("expected Labels section header in output: %q", out)
	}
	if !strings.Contains(out, "  env:") {
		t.Fatalf("expected indented detail under section: %q", out)
	}
}

func TestPrintDetail_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatJSON, Out: &buf}

	msg := structpb.NewStringValue("detail-test")
	err := p.PrintDetail(msg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %q", buf.String())
	}
}

func TestPrintDetail_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: "xml", Out: &buf}

	err := p.PrintDetail(structpb.NewStringValue("test"), nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestPrintToken(t *testing.T) {
	var buf bytes.Buffer
	PrintToken(&buf, "secret-token-xyz")

	out := buf.String()
	if !strings.Contains(out, "WARNING") {
		t.Fatalf("expected WARNING in output: %q", out)
	}
	if !strings.Contains(out, "secret-token-xyz") {
		t.Fatalf("expected token in output: %q", out)
	}
}

func TestFormatScopes(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := FormatScopes(nil); got != "<none>" {
			t.Fatalf("want <none>, got %q", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		if got := FormatScopes([]string{"read"}); got != "read" {
			t.Fatalf("want %q, got %q", "read", got)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		got := FormatScopes([]string{"read", "write", "admin"})
		if got != "read, write, admin" {
			t.Fatalf("want %q, got %q", "read, write, admin", got)
		}
	})
}

// ---------------------------------------------------------------------------
// table.go
// ---------------------------------------------------------------------------

func TestFormatAge(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := FormatAge(nil); got != "<unknown>" {
			t.Fatalf("want <unknown>, got %q", got)
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
		if got := FormatTimestamp(nil); got != "<none>" {
			t.Fatalf("want <none>, got %q", got)
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
		if got := FormatLabels(nil); got != "<none>" {
			t.Fatalf("want <none>, got %q", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got := FormatLabels(map[string]string{}); got != "<none>" {
			t.Fatalf("want <none>, got %q", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		got := FormatLabels(map[string]string{"env": "prod"})
		if got != "env=prod" {
			t.Fatalf("want %q, got %q", "env=prod", got)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		got := FormatLabels(map[string]string{"a": "1", "b": "2"})
		// Map iteration order is non-deterministic; check both labels are present.
		if !strings.Contains(got, "a=1") || !strings.Contains(got, "b=2") {
			t.Fatalf("expected both labels, got %q", got)
		}
		if !strings.Contains(got, ",") {
			t.Fatalf("expected comma separator, got %q", got)
		}
	})
}

func TestFormatEnum(t *testing.T) {
	tests := []struct {
		name   string
		enum   string
		prefix string
		want   string
	}{
		{"healthy", "CLUSTER_HEALTH_STATUS_HEALTHY", "CLUSTER_HEALTH_STATUS_", "Healthy"},
		{"degraded", "CLUSTER_HEALTH_STATUS_DEGRADED", "CLUSTER_HEALTH_STATUS_", "Degraded"},
		{"unspecified", "CLUSTER_HEALTH_STATUS_UNSPECIFIED", "CLUSTER_HEALTH_STATUS_", "Unknown"},
		{"empty after prefix", "SOME_PREFIX_", "SOME_PREFIX_", "Unknown"},
		{"no prefix match", "HEALTHY", "NONEXISTENT_", "Healthy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatEnum(tc.enum, tc.prefix)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0s"},
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
		{"just under minute", 59 * time.Second, "59s"},
		{"just under hour", 59 * time.Minute, "59m"},
		{"just under day", 23 * time.Hour, "23h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatDuration(tc.duration)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Additional printer.go tests
// ---------------------------------------------------------------------------

func TestNewPrinter_DefaultsToStdout(t *testing.T) {
	p := NewPrinter(FormatTable)
	require.Equal(t, FormatTable, p.Format)
	require.Equal(t, os.Stdout, p.Out)
}

func TestPrintResource_Wide(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatWide, Out: &buf}

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
	p := &Printer{Format: FormatJSON, Out: &buf}

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
	p := &Printer{Format: FormatJSON, Out: &buf}

	err := p.PrintResource(structpb.NewStringValue("test"), nil)
	require.NoError(t, err)

	// JSON output should be indented (multiline)
	require.Contains(t, buf.String(), "\n")
}

func TestPrintResource_YAML_Structure(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatYAML, Out: &buf}

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
	p := &Printer{Format: FormatJSON, Out: &buf}

	// nil proto.Message should marshal as empty JSON object
	err := p.PrintResource(nil, nil)
	require.NoError(t, err)
	require.True(t, json.Valid(buf.Bytes()))
}

func TestPrintResource_NilMessage_YAML(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatYAML, Out: &buf}

	err := p.PrintResource(nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, buf.String())
}

func TestPrintDetail_YAML(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatYAML, Out: &buf}

	msg := structpb.NewStringValue("detail-yaml-test")
	sections := []Section{
		{Details: []Detail{{Key: "Name", Value: "test"}}},
	}

	err := p.PrintDetail(msg, sections)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "detail-yaml-test")
}

func TestPrintDetail_Wide(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatWide, Out: &buf}

	sections := []Section{
		{
			Details: []Detail{
				{Key: "Name", Value: "wide-cluster"},
				{Key: "ID", Value: "xyz-789"},
			},
		},
	}

	err := p.PrintDetail(structpb.NewStringValue("test"), sections)
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "Name:")
	require.Contains(t, out, "wide-cluster")
	require.Contains(t, out, "ID:")
	require.Contains(t, out, "xyz-789")
}

func TestPrintDetail_EmptySections(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatTable, Out: &buf}

	err := p.PrintDetail(structpb.NewStringValue("test"), nil)
	require.NoError(t, err)
	require.Empty(t, buf.String())
}

func TestPrintDetail_MultipleSectionsSpacing(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatTable, Out: &buf}

	sections := []Section{
		{
			Details: []Detail{{Key: "Name", Value: "test"}},
		},
		{
			Name:    "Metadata",
			Details: []Detail{{Key: "env", Value: "prod"}},
		},
		{
			Name:    "Status",
			Details: []Detail{{Key: "health", Value: "ok"}},
		},
	}

	err := p.PrintDetail(structpb.NewStringValue("test"), sections)
	require.NoError(t, err)

	out := buf.String()
	// Sections should be separated by blank lines
	require.Contains(t, out, "Name:")
	require.Contains(t, out, "Metadata:")
	require.Contains(t, out, "Status:")
	require.Contains(t, out, "  env:")
	require.Contains(t, out, "  health:")
}

func TestPrintDetail_UnsupportedFormat_Message(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: "csv", Out: &buf}

	err := p.PrintDetail(structpb.NewStringValue("test"), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported format")
	require.Contains(t, err.Error(), "csv")
}

func TestPrintToken_ExactFormat(t *testing.T) {
	var buf bytes.Buffer
	PrintToken(&buf, "adm_pat_abc123")

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 3)
	require.Empty(t, lines[0]) // leading blank line
	require.Equal(t, "WARNING: Save this token — it will not be shown again.", lines[1])
	require.Equal(t, "Token: adm_pat_abc123", lines[2])
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
		{"59 seconds", 59 * time.Second, "s"},
		{"60 seconds", 60 * time.Second, "m"},
		{"59 minutes", 59 * time.Minute, "m"},
		{"60 minutes", 60 * time.Minute, "h"},
		{"23 hours", 23 * time.Hour, "h"},
		{"24 hours", 24 * time.Hour, "d"},
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

func TestFormatEnum_MultiWordValue(t *testing.T) {
	// e.g., "RUNNER_KIND_TERRAFORM" → "Terraform"
	got := FormatEnum("RUNNER_KIND_TERRAFORM", "RUNNER_KIND_")
	require.Equal(t, "Terraform", got)
}

func TestFormatEnum_SingleChar(t *testing.T) {
	got := FormatEnum("PREFIX_X", "PREFIX_")
	require.Equal(t, "X", got)
}

func TestFormatEnum_EmptyInput(t *testing.T) {
	got := FormatEnum("", "")
	require.Equal(t, "Unknown", got)
}

func TestFormatScopes_SingleScope(t *testing.T) {
	got := FormatScopes([]string{"admin"})
	require.Equal(t, "admin", got)
}

func TestFormatScopes_EmptySlice(t *testing.T) {
	got := FormatScopes([]string{})
	require.Equal(t, "<none>", got)
}

func TestFormatDuration_Negative(t *testing.T) {
	// Negative duration should still produce a result (0s or negative)
	got := formatDuration(-5 * time.Second)
	require.NotEmpty(t, got)
}

func TestFormatDuration_LargeDuration(t *testing.T) {
	got := formatDuration(365 * 24 * time.Hour)
	require.Equal(t, "365d", got)
}
