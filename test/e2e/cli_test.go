package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CLI runs the built admiral binary with a per-test environment. Construct
// one per test with newCLI(); Run execs a single invocation and returns a
// fluent Result for chained assertions.
type CLI struct {
	binary string
	env    []string
}

func newCLI() *CLI {
	return &CLI{
		binary: filepath.Join(cliDir, "admiral"),
		env: append(os.Environ(),
			"ADMIRAL_CONFIG_DIR="+sharedConfigDir,
		),
	}
}

// Result is the captured output of a single admiral invocation. Assertion
// methods fatal the test on mismatch so callers can chain without
// per-step error handling.
type Result struct {
	t        *testing.T
	args     []string
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run execs the CLI with args. It does NOT fail on a non-zero exit — chain
// .Succeeds() or assert on ExitCode explicitly for negative-path tests.
func (c *CLI) Run(t *testing.T, args ...string) *Result {
	t.Helper()
	cmd := exec.Command(c.binary, args...)
	cmd.Env = c.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("admiral %s: %v", strings.Join(args, " "), err)
		}
	}
	return &Result{
		t:        t,
		args:     args,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

func (r *Result) label() string { return "admiral " + strings.Join(r.args, " ") }

// Succeeds fails the test if the command exited non-zero.
func (r *Result) Succeeds() *Result {
	r.t.Helper()
	if r.ExitCode != 0 {
		r.t.Fatalf("%s: exit %d\nstdout:\n%s\nstderr:\n%s",
			r.label(), r.ExitCode, r.Stdout, r.Stderr)
	}
	return r
}

// Fails fails the test if the command exited zero. Useful for negative
// paths (e.g. deleting a nonexistent resource).
func (r *Result) Fails() *Result {
	r.t.Helper()
	if r.ExitCode == 0 {
		r.t.Fatalf("%s: expected non-zero exit, got 0\nstdout:\n%s\nstderr:\n%s",
			r.label(), r.Stdout, r.Stderr)
	}
	return r
}

// NoStderr asserts the command succeeded and produced no stderr.
func (r *Result) NoStderr() *Result {
	r.t.Helper()
	r.Succeeds()
	if r.Stderr != "" {
		r.t.Fatalf("%s: expected empty stderr, got:\n%s", r.label(), r.Stderr)
	}
	return r
}

// StdoutContains asserts the command succeeded and stdout contains s.
func (r *Result) StdoutContains(s string) *Result {
	r.t.Helper()
	r.Succeeds()
	if !strings.Contains(r.Stdout, s) {
		r.t.Fatalf("%s: stdout missing %q, got:\n%s", r.label(), s, r.Stdout)
	}
	return r
}

// StdoutNotContains asserts the command succeeded and stdout does not
// contain s.
func (r *Result) StdoutNotContains(s string) *Result {
	r.t.Helper()
	r.Succeeds()
	if strings.Contains(r.Stdout, s) {
		r.t.Fatalf("%s: stdout unexpectedly contained %q\nstdout:\n%s",
			r.label(), s, r.Stdout)
	}
	return r
}

// StderrContains asserts the given substring appears in stderr. Does not
// assert on exit code — some CLIs write informational messages to stderr
// on success (e.g. kubectl-style "No runners found.").
func (r *Result) StderrContains(s string) *Result {
	r.t.Helper()
	if !strings.Contains(r.Stderr, s) {
		r.t.Fatalf("%s: stderr missing %q, got:\n%s", r.label(), s, r.Stderr)
	}
	return r
}

// DecodeJSON asserts the command succeeded and unmarshals stdout into v.
// Use with `-o json` output.
func (r *Result) DecodeJSON(v any) *Result {
	r.t.Helper()
	r.Succeeds()
	if err := json.Unmarshal([]byte(r.Stdout), v); err != nil {
		r.t.Fatalf("%s: decode JSON: %v\nstdout:\n%s", r.label(), err, r.Stdout)
	}
	return r
}
