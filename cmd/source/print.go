package source

import (
	"text/tabwriter"

	"go.admiral.io/cli/internal/output"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

var sourceTypeLabel = map[sourcev1.SourceType]string{
	sourcev1.SourceType_SOURCE_TYPE_GIT:       "GIT",
	sourcev1.SourceType_SOURCE_TYPE_TERRAFORM: "TERRAFORM",
	sourcev1.SourceType_SOURCE_TYPE_HELM:      "HELM",
	sourcev1.SourceType_SOURCE_TYPE_OCI:       "OCI",
	sourcev1.SourceType_SOURCE_TYPE_HTTP:      "HTTP",
}

func formatSourceType(t sourcev1.SourceType) string {
	if s, ok := sourceTypeLabel[t]; ok {
		return s
	}
	return t.String()
}

var testStatusLabel = map[sourcev1.SourceTestStatus]string{
	sourcev1.SourceTestStatus_SOURCE_TEST_STATUS_SUCCESS: "SUCCESS",
	sourcev1.SourceTestStatus_SOURCE_TEST_STATUS_FAILURE: "FAILURE",
}

func formatTestStatus(s *sourcev1.SourceTestStatus) string {
	if s == nil {
		return "-"
	}
	if v, ok := testStatusLabel[*s]; ok {
		return v
	}
	return s.String()
}

// printSourceRow writes a single-row summary (header + values) to w.
func printSourceRow(w *tabwriter.Writer, s *sourcev1.Source) {
	output.Writeln(w, "NAME\tTYPE\tURL\tAGE")
	output.Writef(w, "%s\t%s\t%s\t%s\n",
		s.Name,
		formatSourceType(s.Type),
		s.Url,
		output.FormatAge(s.CreatedAt),
	)
}