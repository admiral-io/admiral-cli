package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/admiraltest"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/version"
)

// The shape sweep runs every command against admiraltest in every shape it
// sends. A command that panics on a shape the server can send fails here
// instead of in front of a user.

// sweepOffline are the commands that never reach the API.
var sweepOffline = map[string]string{
	"auth login":   "opens a browser",
	"auth logout":  "local only",
	"completion":   "local only",
	"config get":   "local only",
	"config list":  "local only",
	"config set":   "local only",
	"config unset": "local only",
	"version":      "local only",
}

// sweepDocuments are the commands whose table output is a document in its
// own format, not a view, so the blank-line rule does not apply.
var sweepDocuments = map[string]string{
	"changeset values": "prints a YAML document",
	"env values":       "prints a YAML document",
	"state download":   "prints the state file",
	"run logs":         "prints the agent's transcript",
}

// sweepArgs is one invocation of each command that reaches the API, with
// arguments it accepts. files holds paths to fixtures it reads.
func sweepArgs(files map[string]string) map[string][]string {
	const (
		cs  = "cs-000000000001"
		run = "run-000000000001"
	)
	return map[string][]string{
		"agent create":         {"agent", "create", "x", "--kind", "kubernetes"},
		"agent delete":         {"agent", "delete", "x"},
		"agent describe":       {"agent", "describe", "x"},
		"agent enrollment-key": {"agent", "enrollment-key", "x"},
		"agent get":            {"agent", "get", "x"},
		"agent grant":          {"agent", "grant", "x", "app:x"},
		"agent list":           {"agent", "list"},
		"agent revoke":         {"agent", "revoke", "x", "org"},
		"agent update":         {"agent", "update", "x", "--name", "y"},

		"app create":   {"app", "create", "x"},
		"app delete":   {"app", "delete", "x", "--yes"},
		"app describe": {"app", "describe", "x"},
		"app get":      {"app", "get", "x"},
		"app list":     {"app", "list"},
		"app update":   {"app", "update", "x", "--description", "y"},

		"auth status": {"auth", "status"},

		"changeset add":      {"changeset", "add", cs, "x:v1", "--values", files["values"]},
		"changeset apply":    {"changeset", "apply", cs, "--yes", "--no-wait"},
		"changeset approve":  {"changeset", "approve", cs, "--yes"},
		"changeset create":   {"changeset", "create", "x/x", "--title", "t"},
		"changeset describe": {"changeset", "describe", cs},
		"changeset diff":     {"changeset", "diff", cs},
		"changeset discard":  {"changeset", "discard", cs, "--yes"},
		"changeset get":      {"changeset", "get", cs},
		"changeset list":     {"changeset", "list", "x/x"},
		"changeset plan":     {"changeset", "plan", cs, "--no-wait"},
		"changeset remove":   {"changeset", "remove", cs, "x"},
		"changeset revert":   {"changeset", "revert", cs},
		"changeset set":      {"changeset", "set", cs, "x.a=1"},
		"changeset unset":    {"changeset", "unset", cs, "x.a"},
		"changeset values":   {"changeset", "values", cs, "x"},

		"component deprecate":    {"component", "deprecate", "x:v1", "--reason", "r", "--yes"},
		"component get":          {"component", "get", "x"},
		"component import":       {"component", "import", "x", "--repo", "https://example.com/charts", "--version", "1.0.0"},
		"component list":         {"component", "list"},
		"component publish":      {"component", "publish", files["manifests"], "--name", "x", "--kind", "manifests"},
		"component restore":      {"component", "restore", "x:v1", "--reason", "r", "--yes"},
		"component revoke":       {"component", "revoke", "x:v1", "--reason", "r", "--yes"},
		"component tag":          {"component", "tag", "x:v2", "--digest", "sha256:" + strings.Repeat("ab", 32)},
		"component untag":        {"component", "untag", "x:v1", "--yes"},
		"component version get":  {"component", "version", "get", "x:v1"},
		"component version list": {"component", "version", "list", "x"},

		"apikey create":                  {"apikey", "create", "x", "--scope", "state:write"},
		"apikey list":                    {"apikey", "list"},
		"apikey revoke":                  {"apikey", "revoke", "x", "--yes"},
		"state download":                 {"state", "download", "stv-1"},
		"state versions":                 {"state", "versions", "x/x", "x"},
		"credential create basic-auth":   {"credential", "create", "basic-auth", "x", "--username", "u", "--password-stdin"},
		"credential create bearer-token": {"credential", "create", "bearer-token", "x", "--token-stdin", "--allowed-host", "example.com"},
		"credential create github-app":   {"credential", "create", "github-app", "x", "--app-id", "1", "--installation-id", "2", "--private-key-file", files["key"]},
		"credential create ssh-key":      {"credential", "create", "ssh-key", "x", "--private-key-file", files["key"]},
		"credential delete":              {"credential", "delete", "x", "--yes"},
		"credential get":                 {"credential", "get", "x"},
		"credential list":                {"credential", "list"},
		"credential rotate":              {"credential", "rotate", "x", "--token-stdin"},
		"credential update":              {"credential", "update", "x", "--description", "y"},

		"env create":   {"env", "create", "x/x"},
		"env delete":   {"env", "delete", "x/x", "--yes"},
		"env describe": {"env", "describe", "x/x"},
		"env get":      {"env", "get", "x/x"},
		"env list":     {"env", "list", "x"},
		"env update":   {"env", "update", "x/x", "--description", "y"},
		"env values":   {"env", "values", "x/x", "x"},

		"run accept":  {"run", "accept", run, "--reason", "r", "--yes"},
		"run apply":   {"run", "apply", run, "--wait=false"},
		"run approve": {"run", "approve", run},
		"run cancel":  {"run", "cancel", run},
		"run diff":    {"run", "diff", run},
		"run get":     {"run", "get", run},
		"run list":    {"run", "list", "x/x"},
		"run logs":    {"run", "logs", run},
	}
}

func sweepFiles(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	write("manifests/cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n")
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return map[string]string{
		"values":    write("values.yaml", "a: 1\n"),
		"key":       write("key.pem", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))),
		"manifests": filepath.Join(dir, "manifests"),
	}
}

func leafCommands(c *cobra.Command) []string {
	var out []string
	if !c.HasSubCommands() && c.Runnable() {
		out = append(out, strings.TrimPrefix(c.CommandPath(), "admiral "))
	}
	for _, s := range c.Commands() {
		if s.Name() == "help" {
			continue
		}
		out = append(out, leafCommands(s)...)
	}
	return out
}

// Every command is in the sweep or says why it is not, so a new command
// cannot skip it.
func TestShapeSweepCoversEveryCommand(t *testing.T) {
	root, err := newRootCmd(version.GetVersion(), func(int) {})
	require.NoError(t, err)
	args := sweepArgs(map[string]string{})
	var missing []string
	for _, c := range leafCommands(root.cmd) {
		if _, ok := args[c]; !ok && sweepOffline[c] == "" {
			missing = append(missing, c)
		}
	}
	require.Empty(t, missing, "add these commands to sweepArgs, or to sweepOffline with the reason")
	for c := range args {
		require.NotContains(t, sweepOffline, c)
	}
}

type sweepResult struct {
	stdout, stderr string
	err            error
	panicked       string
}

func sweepRun(t *testing.T, addr string, args []string) (r sweepResult) {
	t.Helper()
	root, err := newRootCmd(version.GetVersion(), func(int) {})
	require.NoError(t, err)
	var out, errOut bytes.Buffer
	root.cmd.SetOut(&out)
	root.cmd.SetErr(&errOut)
	root.cmd.SetIn(strings.NewReader("s3cret\n"))
	root.cmd.SetArgs(append([]string{
		"--config-dir", t.TempDir(), "--server", addr, "--plaintext", "--no-input",
	}, args...))
	// A command that polls, such as a wait or a followed log, stops here;
	// the sweep is after panics, not after what a wait settles on.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	func() {
		defer func() {
			if p := recover(); p != nil {
				r.panicked = fmt.Sprintf("%v\n%s", p, debug.Stack())
			}
		}()
		r.err = root.cmd.ExecuteContext(ctx)
	}()
	r.stdout, r.stderr = out.String(), errOut.String()
	return r
}

// stateServer stands in for admiral-state's history routes, which are plain
// HTTP and not in the Admiral API admiraltest serves.
func stateServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			_, _ = io.WriteString(w, `{"version":4,"serial":3}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"stv-1","type":"state-versions","attributes":{"serial":3,"size":2048,"created-at":"2026-10-25T12:00:00Z","status":"finalized"},"relationships":{"created-by":{"data":{"type":"users","id":"usr-1"}}}},`+
			`{"id":"stv-0","type":"state-versions","attributes":{"serial":2,"size":10,"forced":true,"created-at":"2026-10-24T12:00:00Z","status":"finalized"},"relationships":{"created-by":{"data":{"type":"jobs","id":"job-1"},"meta":{"attempt":1}}}}],"meta":{"pagination":{"current-page":1,"next-page":null}}}`)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestShapeSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("the shape sweep runs every command against every response shape")
	}
	t.Setenv(credentials.EnvAPIKey, "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG")
	t.Setenv("ADMIRAL_NO_INPUT", "1")
	srv := admiraltest.Start(t)
	t.Setenv(client.EnvStateHost, stateServer(t))
	invocations := sweepArgs(sweepFiles(t))
	names := make([]string, 0, len(invocations))
	for n := range invocations {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			reached, succeeded := false, false
			var refused, failed error
			for _, s := range admiraltest.Shapes() {
				srv.Respond(s)
				for _, format := range []string{"table", "wide", "json", "yaml", "name"} {
					args := append([]string{"-o", format}, invocations[name]...)
					r := sweepRun(t, srv.Addr(), args)
					if r.panicked != "" {
						t.Fatalf("admiral %s with a %s response panicked:\n%s", strings.Join(args, " "), s, r.panicked)
					}
					if format == "table" {
						switch {
						case r.err == nil:
							reached, succeeded = true, true
						case cmderr.Code(r.err) == cmderr.ExitUsage:
							refused = r.err
						default:
							reached, failed = true, r.err
						}
					}
					if r.err == nil && (format == "table" || format == "wide") && sweepDocuments[name] == "" {
						for _, out := range []string{r.stdout, r.stderr} {
							for _, v := range blankLineViolations(out) {
								t.Errorf("admiral %s with a %s response: %s\n%s", strings.Join(args, " "), s, v, out)
							}
						}
					}
					if r.err == nil && format == "json" {
						if trimmed := strings.TrimSpace(r.stdout); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
							require.True(t, json.Valid([]byte(trimmed)),
								"admiral %s with a %s response printed invalid JSON:\n%s", strings.Join(args, " "), s, trimmed)
						}
					}
				}
			}
			// A command may refuse some responses as usage errors, as rotate
			// does for a type its flags do not fit, but one refused every
			// time never reaches the API and its arguments need fixing.
			require.True(t, reached, "admiral %s is refused as a usage error for every response: %v",
				strings.Join(invocations[name], " "), refused)
			// A command that fails on every shape stops before its own code,
			// at a lookup or a check, and the sweep tests nothing of it.
			require.True(t, succeeded, "admiral %s fails for every response, last with: %v",
				strings.Join(invocations[name], " "), failed)
		})
	}
}

// blankLineViolations checks output against the blank-line rule in
// CLAUDE.md: a blank line before each top-level section and each table,
// except as the first thing in a block; never two in a row, never one
// first, and never one holding only spaces.
func blankLineViolations(out string) []string {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	blank := func(i int) bool { return i < 0 || lines[i] == "" }
	heading := func(l string) bool {
		t := strings.TrimSpace(l)
		return strings.HasSuffix(t, ":") && !strings.Contains(t, "  ")
	}
	underline := func(l string) bool {
		t := strings.TrimSpace(l)
		return t != "" && strings.Trim(t, "- ") == ""
	}
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
	var out2 []string
	for i, l := range lines {
		switch {
		case l != "" && strings.TrimSpace(l) == "":
			out2 = append(out2, fmt.Sprintf("line %d is blank but holds spaces", i+1))
		case l == "" && i == 0 && len(lines) > 1:
			out2 = append(out2, "the output opens with a blank line")
		case l == "" && i > 0 && lines[i-1] == "":
			out2 = append(out2, fmt.Sprintf("lines %d and %d are both blank", i, i+1))
		case i+1 < len(lines) && underline(lines[i+1]) && !underline(l) && i > 0 && !blank(i-1) && !heading(lines[i-1]):
			out2 = append(out2, fmt.Sprintf("the table at line %d follows %q with no blank line", i+1, lines[i-1]))
		case indent(l) == 0 && heading(l) && i > 0 && !blank(i-1) &&
			i+1 < len(lines) && indent(lines[i+1]) > 0:
			out2 = append(out2, fmt.Sprintf("the section %q at line %d has no blank line before it", l, i+1))
		}
	}
	return out2
}

func TestBlankLineViolations(t *testing.T) {
	good := "ID:      run-1\nStatus:  applied\n\nPlan:\n  Digest:  sha256:1\n\n" +
		"  Component  Changes\n  ---------  -------\n  api        +1\n\nEvents:\n  Age  Type\n  ---  ----\n  3h   Normal\n\n" +
		"To see the changes, run: admiral run diff run-1\n"
	require.Empty(t, blankLineViolations(good))

	for name, bad := range map[string]string{
		"a table straight after fields":   "Plan:\n  Digest:  sha256:1\n  Component  Changes\n  ---------  -------\n",
		"a section straight after fields": "ID:  run-1\nPlan:\n  Digest:  sha256:1\n",
		"a blank line first":              "\nID:  run-1\n",
		"two blank lines":                 "ID:  run-1\n\n\nPlan:\n  Digest:  sha256:1\n",
		"a blank line holding spaces":     "ID:  run-1\n  \nPlan:\n  Digest:  sha256:1\n",
	} {
		require.NotEmpty(t, blankLineViolations(bad), name)
	}
}
