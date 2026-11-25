package component

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func TestImportRefusesLocallyWhatItCan(t *testing.T) {
	_, err := run(t, "import", "Not_Valid", "--source", "jetstack", "--version", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase letters, digits and hyphens")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "import")
	require.EqualError(t, err, "missing argument: import <name> --version VERSION [--source SOURCE]")

	bad := filepath.Join(t.TempDir(), "baseline.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("- a\n- b\n"), 0o644))
	_, err = run(t, "import", "cert-manager", "--source", "jetstack", "--version", "1", "--baseline-values", bad)
	require.ErrorContains(t, err, "values must be a map at the top level")
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	_, err = run(t, "import", "cert-manager", "--source", "jetstack", "--version", "1", "--baseline-values", filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorContains(t, err, "cannot read --baseline-values")

	for _, gone := range []string{"--repo", "--from", "--ref", "--path", "--credential"} {
		_, err = run(t, "import", "cert-manager", gone, "x")
		require.ErrorContains(t, err, "unknown flag: "+gone, "the raw import flags are gone")
	}
}

func TestReadBaselineIsJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), "baseline.yaml")
	require.NoError(t, os.WriteFile(file, []byte("replicas: 3\ningress:\n  enabled: true\n"), 0o644))
	got, err := readBaseline(file)
	require.NoError(t, err)
	assert.JSONEq(t, `{"replicas": 3, "ingress": {"enabled": true}}`, got)
}

func TestReportPublishedListsEachFindingOnceAndInfoOnlyWhenVerbose(t *testing.T) {
	ver := &registryv1.Version{Digest: "sha256:3902c9ddbe75", Findings: []*registryv1.Finding{
		{Severity: registryv1.FindingSeverity_FINDING_SEVERITY_INFO, Source: "image-ref", Message: "alpine:3.11"},
		{Severity: registryv1.FindingSeverity_FINDING_SEVERITY_INFO, Source: "image-ref", Message: "alpine:3.11"},
		{Severity: registryv1.FindingSeverity_FINDING_SEVERITY_INFO, Source: "image-ref", Message: "curlimages/curl:7.69.0"},
		{Severity: registryv1.FindingSeverity_FINDING_SEVERITY_HIGH, Source: "lint", Message: "runs as root"},
		{Severity: registryv1.FindingSeverity_FINDING_SEVERITY_HIGH, Source: "lint", Message: "runs as root"},
	}}
	report := func(verbose bool) string {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("verbose", verbose, "")

		var errOut bytes.Buffer
		cmd.SetErr(&errOut)
		reportPublished(cmd, output.NewPrinter(cmd, output.FormatTable), &registryv1.Component{Name: "podinfo"}, ver, false)

		return errOut.String()
	}

	quiet := report(false)
	assert.Equal(t, 1, strings.Count(quiet, "runs as root"), "a finding is listed once")
	assert.NotContains(t, quiet, "alpine")
	assert.Contains(t, quiet, "2 info finding(s); --verbose lists them", "duplicates are not counted twice")

	loud := report(true)
	assert.Equal(t, 1, strings.Count(loud, "alpine:3.11"))
	assert.Contains(t, loud, "curlimages/curl:7.69.0")
	assert.NotContains(t, loud, "--verbose lists them")
}
