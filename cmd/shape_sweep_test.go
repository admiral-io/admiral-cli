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

		"changeset add":     {"changeset", "add", cs, "x:v1", "--values", files["values"]},
		"changeset apply":   {"changeset", "apply", cs, "--yes", "--no-wait"},
		"changeset approve": {"changeset", "approve", cs, "--yes"},
		"changeset create":  {"changeset", "create", "x/x", "--title", "t"},
		"changeset diff":    {"changeset", "diff", cs},
		"changeset discard": {"changeset", "discard", cs, "--yes"},
		"changeset get":     {"changeset", "get", cs},
		"changeset list":    {"changeset", "list", "x/x"},
		"changeset plan":    {"changeset", "plan", cs, "--no-wait"},
		"changeset remove":  {"changeset", "remove", cs, "x"},
		"changeset revert":  {"changeset", "revert", cs},
		"changeset set":     {"changeset", "set", cs, "x.a=1"},
		"changeset unset":   {"changeset", "unset", cs, "x.a"},
		"changeset values":  {"changeset", "values", cs, "x"},

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

func TestShapeSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("the shape sweep runs every command against every response shape")
	}
	t.Setenv(credentials.EnvAPIKey, "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG")
	t.Setenv("ADMIRAL_NO_INPUT", "1")
	srv := admiraltest.Start(t)
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
