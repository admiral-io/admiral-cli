package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// CLI runs the built admiral binary with a per-test environment: its own
// config dir and none of the developer's ADMIRAL_* variables. Run execs a
// single invocation and returns a fluent Result for chained assertions.
type CLI struct {
	t         *testing.T
	configDir string
	env       []string
	stdin     string
}

// newCLI is a signed-out CLI with an empty config dir.
func newCLI(t *testing.T) *CLI {
	t.Helper()
	dir := t.TempDir()
	return &CLI{
		t:         t,
		configDir: dir,
		env:       append(scrubbedEnv(), "ADMIRAL_CONFIG_DIR="+dir),
	}
}

// newServerCLI is a CLI pointed at the live stack and authenticated with
// its API key through the environment, so no secret lands in the config
// dir. It skips the test when no stack is configured.
func newServerCLI(t *testing.T) *CLI {
	t.Helper()
	_, apiKey := serverStack(t)
	return newServerCLIWithKey(t, apiKey)
}

// newServerCLIWithKey is newServerCLI presenting apiKey instead of the
// stack's own, for the rejected-credential path.
func newServerCLIWithKey(t *testing.T, apiKey string) *CLI {
	t.Helper()
	server, _ := serverStack(t)
	c := newCLI(t)
	c.env = append(c.env, "ADMIRAL_SERVER="+server, "ADMIRAL_API_KEY="+apiKey)
	if os.Getenv("ADMIRAL_PLAINTEXT") != "" {
		c.Run("config", "set", "plaintext", "true").Succeeds()
	}
	return c
}

// WithStdin returns a copy of c whose next Run feeds in on stdin.
func (c *CLI) WithStdin(in string) *CLI {
	cp := *c
	cp.stdin = in
	return &cp
}

// Result is the captured output of a single admiral invocation. Assertion
// methods fatal the test on mismatch so callers can chain without
// per-step error handling. Succeeds and Exits pin the exit code; the
// content assertions check only what they name, so a negative path reads
// Exits(2).StderrContains(...).
type Result struct {
	t        *testing.T
	args     []string
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run execs the CLI with args. It does NOT fail on a non-zero exit — chain
// .Succeeds() or .Exits(code) explicitly for negative-path tests.
func (c *CLI) Run(args ...string) *Result {
	c.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = c.env
	cmd.Stdin = strings.NewReader(c.stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = ee.ExitCode()
		} else {
			c.t.Fatalf("admiral %s: %v", strings.Join(args, " "), err)
		}
	}
	return &Result{
		t:        c.t,
		args:     args,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

func (r *Result) label() string { return "admiral " + strings.Join(r.args, " ") }

func (r *Result) dump() string {
	return r.label() + ": exit " + strconv.Itoa(r.ExitCode) + "\nstdout:\n" + r.Stdout + "\nstderr:\n" + r.Stderr
}

// Succeeds fails the test if the command exited non-zero.
func (r *Result) Succeeds() *Result {
	r.t.Helper()
	return r.Exits(0)
}

// Exits fails the test unless the command exited with code.
func (r *Result) Exits(code int) *Result {
	r.t.Helper()
	if r.ExitCode != code {
		r.t.Fatalf("%s: expected exit %d\n%s", r.label(), code, r.dump())
	}
	return r
}

// NoStderr asserts the command produced no stderr.
func (r *Result) NoStderr() *Result {
	r.t.Helper()
	if r.Stderr != "" {
		r.t.Fatalf("%s: expected empty stderr, got:\n%s", r.label(), r.Stderr)
	}
	return r
}

// StdoutIs asserts stdout is exactly s.
func (r *Result) StdoutIs(s string) *Result {
	r.t.Helper()
	if r.Stdout != s {
		r.t.Fatalf("%s: stdout is %q, want %q", r.label(), r.Stdout, s)
	}
	return r
}

// StdoutContains asserts stdout contains s.
func (r *Result) StdoutContains(s string) *Result {
	r.t.Helper()
	if !strings.Contains(r.Stdout, s) {
		r.t.Fatalf("%s: stdout missing %q, got:\n%s", r.label(), s, r.Stdout)
	}
	return r
}

// StdoutNotContains asserts stdout does not contain s.
func (r *Result) StdoutNotContains(s string) *Result {
	r.t.Helper()
	if strings.Contains(r.Stdout, s) {
		r.t.Fatalf("%s: stdout unexpectedly contained %q\nstdout:\n%s",
			r.label(), s, r.Stdout)
	}
	return r
}

// StderrContains asserts the given substring appears in stderr. Some
// commands write informational messages to stderr on success
// (kubectl-style "No applications found."), so this says nothing about
// the exit code either way.
func (r *Result) StderrContains(s string) *Result {
	r.t.Helper()
	if !strings.Contains(r.Stderr, s) {
		r.t.Fatalf("%s: stderr missing %q, got:\n%s", r.label(), s, r.Stderr)
	}
	return r
}

// DecodeJSON asserts the command succeeded and unmarshals stdout into v.
// Use with `-o json` output; it implies Succeeds because a failed command
// has no JSON to decode.
func (r *Result) DecodeJSON(v any) *Result {
	r.t.Helper()
	r.Succeeds()
	if err := json.Unmarshal([]byte(r.Stdout), v); err != nil {
		r.t.Fatalf("%s: decode JSON: %v\nstdout:\n%s", r.label(), err, r.Stdout)
	}
	return r
}
