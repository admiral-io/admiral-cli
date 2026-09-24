package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/iostreams"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/version"
)

const hangHelperEnv = "ADMIRAL_TEST_HANG_HELPER"

// TestHangHelper is the child side of TestSecondInterruptKills: it runs the
// root command with a subcommand that ignores its context, so only a signal
// with default handling can end it. It does nothing unless re-executed by
// the parent with hangHelperEnv set.
func TestHangHelper(t *testing.T) {
	if os.Getenv(hangHelperEnv) == "" {
		t.Skip("helper for TestSecondInterruptKills")
	}
	root, err := newRootCmd(version.GetVersion(), os.Exit)
	if err != nil {
		panic(err)
	}
	root.cmd.AddCommand(&cobra.Command{
		Use: "hang",
		RunE: func(*cobra.Command, []string) error {
			os.Stdout.WriteString("ready\n") //nolint:errcheck // test helper
			time.Sleep(30 * time.Second)
			return errors.New("hang was not interrupted")
		},
	})
	root.Execute([]string{"hang"})
	os.Exit(0)
}

// The first Ctrl-C cancels the context; a second one must end the process
// even when the command is not watching the context.
func TestSecondInterruptKills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}

	child := exec.Command(os.Args[0], "-test.run=^TestHangHelper$")
	child.Env = append(os.Environ(), hangHelperEnv+"=1")
	child.Stderr = os.Stderr
	stdout, err := child.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, child.Start())

	// Wait for the command to be inside RunE before signaling.
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)

	require.NoError(t, child.Process.Signal(os.Interrupt))
	time.Sleep(200 * time.Millisecond) // let the context cancel and stop() run
	require.NoError(t, child.Process.Signal(os.Interrupt))

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "the child should die from the signal, not exit cleanly")
		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		require.True(t, ok)
		require.True(t, ws.Signaled(), "expected death by signal, got status %v", ws)
		require.Equal(t, syscall.SIGINT, ws.Signal())
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		t.Fatal("the child survived a second SIGINT")
	}
}

// runRoot executes the root command in-process with args, returning what
// it wrote and the exit code it asked for (-1 when it did not exit). A
// fresh config dir keeps the developer's own config.json out of the run,
// and the environment variables the root reads are cleared unless env
// sets them.
func runRoot(t *testing.T, args []string, env map[string]string) (root *rootCmd, stdout, stderr string, code int) {
	t.Helper()
	for _, k := range []string{envServer, envAuthServer, envClientID, envTimeout, "ADMIRAL_NO_INPUT"} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	code = -1
	root, err := newRootCmd(version.GetVersion(), func(c int) { code = c })
	require.NoError(t, err)
	var out, errOut bytes.Buffer
	root.cmd.SetOut(&out)
	root.cmd.SetErr(&errOut)
	root.Execute(append([]string{"--config-dir", t.TempDir()}, args...))
	return root, out.String(), errOut.String(), code
}

// runRootWith adds a subcommand that returns err and runs it, to drive
// Execute's error reporting with a known error.
func runRootWith(t *testing.T, fail error) (stderr string, code int) {
	t.Helper()
	for _, k := range []string{envServer, envAuthServer, envClientID, envTimeout, "ADMIRAL_NO_INPUT"} {
		t.Setenv(k, "")
	}
	code = -1
	root, err := newRootCmd(version.GetVersion(), func(c int) { code = c })
	require.NoError(t, err)
	root.cmd.AddCommand(&cobra.Command{
		Use:  "probe",
		RunE: func(*cobra.Command, []string) error { return fail },
	})
	var errOut bytes.Buffer
	root.cmd.SetOut(io.Discard)
	root.cmd.SetErr(&errOut)
	root.Execute([]string{"--config-dir", t.TempDir(), "probe"})
	return errOut.String(), code
}

func writeConfig(t *testing.T, dir string, settings map[string]string) {
	t.Helper()
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), data, 0600))
}

func TestRoot_ServerPrecedence(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		env    string
		config string
		want   string
	}{
		{"flag beats env and config", "flag:1", "env:1", "config:1", "flag:1"},
		{"env beats config", "", "env:1", "config:1", "env:1"},
		{"config beats default", "", "", "config:1", "config:1"},
		{"built-in default", "", "", "", defaultServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.config != "" {
				writeConfig(t, dir, map[string]string{"server": tc.config})
			}
			args := []string{"--config-dir", dir, "version"}
			if tc.flag != "" {
				args = append([]string{"--server", tc.flag}, args...)
			}
			// runRoot prepends its own --config-dir; the later one wins.
			root, _, stderr, code := runRoot(t, args, map[string]string{envServer: tc.env})
			require.Equal(t, -1, code, stderr)
			require.Equal(t, tc.want, root.clientOpts.ServerAddr)
		})
	}
}

func TestRoot_AuthServerAndClientIDFromEnv(t *testing.T) {
	root, _, _, code := runRoot(t, []string{"version"}, map[string]string{
		envAuthServer: "https://idp.example.test",
		envClientID:   "cli-test",
	})
	require.Equal(t, -1, code)
	require.Equal(t, "https://idp.example.test", root.clientOpts.Issuer)
	require.Equal(t, "cli-test", root.clientOpts.ClientID)

	root, _, _, code = runRoot(t, []string{"--auth-server", "https://flag.example.test", "--client-id", "cli-flag", "version"}, map[string]string{
		envAuthServer: "https://idp.example.test",
		envClientID:   "cli-test",
	})
	require.Equal(t, -1, code)
	require.Equal(t, "https://flag.example.test", root.clientOpts.Issuer)
	require.Equal(t, "cli-flag", root.clientOpts.ClientID)
}

func TestRoot_Timeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		root, _, _, code := runRoot(t, []string{"version"}, nil)
		require.Equal(t, -1, code)
		require.Equal(t, client.DefaultTimeout, root.clientOpts.Timeout)
	})
	t.Run("env", func(t *testing.T) {
		root, _, _, code := runRoot(t, []string{"version"}, map[string]string{envTimeout: "5s"})
		require.Equal(t, -1, code)
		require.Equal(t, 5*time.Second, root.clientOpts.Timeout)
	})
	t.Run("flag beats env", func(t *testing.T) {
		root, _, _, code := runRoot(t, []string{"--timeout", "7s", "version"}, map[string]string{envTimeout: "5s"})
		require.Equal(t, -1, code)
		require.Equal(t, 7*time.Second, root.clientOpts.Timeout)
	})
	t.Run("invalid env is a usage error", func(t *testing.T) {
		_, _, stderr, code := runRoot(t, []string{"version"}, map[string]string{envTimeout: "soon"})
		require.Equal(t, cmderr.ExitUsage, code)
		require.Contains(t, stderr, `invalid ADMIRAL_TIMEOUT "soon"`)
	})
	t.Run("invalid flag is a usage error", func(t *testing.T) {
		_, _, stderr, code := runRoot(t, []string{"--timeout", "soon", "version"}, nil)
		require.Equal(t, cmderr.ExitUsage, code)
		require.Contains(t, stderr, `invalid argument "soon" for "--timeout"`)
		require.Contains(t, stderr, "--help' for usage.")
	})
}

func TestRoot_ConfigSettings(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, map[string]string{"insecure": "true", "plaintext": "true", "output": "json"})

	root, _, _, code := runRoot(t, []string{"--config-dir", dir, "version"}, nil)
	require.Equal(t, -1, code)
	require.True(t, root.clientOpts.Insecure)
	require.True(t, root.clientOpts.PlainText)
	require.Equal(t, output.FormatJSON, root.clientOpts.OutputFormat)

	// Flags still win over the file.
	root, _, _, code = runRoot(t, []string{"--config-dir", dir, "-o", "yaml", "--insecure=false", "version"}, nil)
	require.Equal(t, -1, code)
	require.False(t, root.clientOpts.Insecure)
	require.Equal(t, output.FormatYAML, root.clientOpts.OutputFormat)
}

func TestRoot_WarnsAboutTokenInConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, map[string]string{"token": "admp_stale"})

	_, _, stderr, code := runRoot(t, []string{"--config-dir", dir, "version"}, nil)
	require.Equal(t, -1, code)
	require.Contains(t, stderr, "config.json contains a 'token' entry that is no longer read")
	require.NotContains(t, stderr, "admp_stale", "the stale value itself is not echoed")

	_, _, stderr, _ = runRoot(t, []string{"version"}, nil)
	require.Empty(t, stderr, "no warning without the entry")
}

func TestRoot_InvalidOutputFormat(t *testing.T) {
	_, _, stderr, code := runRoot(t, []string{"-o", "xml", "version"}, nil)
	require.Equal(t, cmderr.ExitUsage, code)
	require.Contains(t, stderr, "Error: ")
	require.Contains(t, stderr, "xml")
}

func TestRoot_UnknownFlagAndCommand(t *testing.T) {
	_, _, stderr, code := runRoot(t, []string{"--bogus", "version"}, nil)
	require.Equal(t, cmderr.ExitUsage, code)
	require.Contains(t, stderr, "unknown flag: --bogus")
	require.Contains(t, stderr, "Run 'admiral --help' for usage.")

	_, _, stderr, code = runRoot(t, []string{"bogus"}, nil)
	require.Equal(t, cmderr.ExitUsage, code)
	require.Contains(t, stderr, `unknown command "bogus" for "admiral"`)
	require.Contains(t, stderr, "Run 'admiral --help' for usage.")

	// The same under a noun, attributed to the noun.
	_, _, stderr, code = runRoot(t, []string{"app", "bogus"}, nil)
	require.Equal(t, cmderr.ExitUsage, code)
	require.Contains(t, stderr, `unknown command "bogus" for "admiral app"`)
	require.Contains(t, stderr, "Run 'admiral app --help' for usage.")

	// A bare noun prints its help and succeeds.
	_, stdout, _, code := runRoot(t, []string{"app"}, nil)
	require.Equal(t, -1, code)
	require.Contains(t, stdout, "Available Commands:")
}

func TestRoot_VersionAndCompletion(t *testing.T) {
	_, stdout, _, code := runRoot(t, []string{"version"}, nil)
	require.Equal(t, -1, code)
	require.Equal(t, version.GetVersion().String(), stdout)

	_, stdout, _, code = runRoot(t, []string{"--version"}, nil)
	require.Equal(t, -1, code)
	require.Contains(t, stdout, version.GetVersion().String())

	_, stdout, _, code = runRoot(t, []string{"completion", "zsh"}, nil)
	require.Equal(t, -1, code)
	require.Contains(t, stdout, "#compdef admiral")

	_, _, stderr, code := runRoot(t, []string{"completion", "tcsh"}, nil)
	require.Equal(t, cmderr.ExitUsage, code, stderr)
	require.Contains(t, stderr, `unknown shell "tcsh"`)
}

// Execute maps an error to the exit code and remedy the contract promises.
func TestRoot_ExitCodes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		code     int
		contains []string
	}{
		{"usage", cmderr.Usage("bad thing"), cmderr.ExitUsage, []string{"Error: bad thing"}},
		{"usage with hint", cmderr.UsageHint("Try --force.", "needs a terminal"), cmderr.ExitUsage, []string{"Error: needs a terminal", "Try --force."}},
		{"not signed in", credentials.ErrNotAuthenticated, cmderr.ExitAuth, []string{"Error: not signed in", "Run 'admiral auth login' to sign in, or set ADMIRAL_API_KEY."}},
		{"session expired", credentials.ErrSessionExpired, cmderr.ExitAuth, []string{"Run 'admiral auth login'"}},
		{"server unauthenticated", status.Error(codes.Unauthenticated, "key revoked"), cmderr.ExitAuth, []string{"Error: not signed in: key revoked", "Run 'admiral auth login'"}},
		{"permission denied", status.Error(codes.PermissionDenied, "scope"), cmderr.ExitError, []string{"Error: permission denied: scope", cmderr.ScopeHint}},
		{"plain error", errors.New("boom"), cmderr.ExitError, []string{"Error: boom"}},
		{"required flag", errors.New(`required flag(s) "app" not set`), cmderr.ExitUsage, []string{"required flag"}},
		{"exclusive flags", errors.New("if any flags in the group [all page-token] are set none of the others can be; [all page-token] were all set"), cmderr.ExitUsage, []string{"[all page-token]"}},
		{"canceled", context.Canceled, cmderr.ExitInterrupted, []string{"Interrupted."}},
		{"wrapped canceled", fmt.Errorf("prompt canceled: %w", context.Canceled), cmderr.ExitInterrupted, []string{"Interrupted."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stderr, code := runRootWith(t, tc.err)
			require.Equal(t, tc.code, code, stderr)
			for _, s := range tc.contains {
				require.Contains(t, stderr, s)
			}
		})
	}

	// A command that already reported its failure sets the exit code and
	// the root prints nothing on top of it.
	t.Run("reported", func(t *testing.T) {
		stderr, code := runRootWith(t, cmderr.Reported(credentials.ErrSessionExpired))
		require.Equal(t, cmderr.ExitAuth, code)
		require.Empty(t, stderr)
	})

	t.Run("success does not exit", func(t *testing.T) {
		stderr, code := runRootWith(t, nil)
		require.Equal(t, -1, code)
		require.Empty(t, stderr)
	})
}

func TestRoot_WarnsAboutCorruptConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"server": `), 0600))

	_, _, stderr, code := runRoot(t, []string{"--config-dir", dir, "version"}, nil)
	require.Equal(t, -1, code, "a corrupt file does not stop the command")
	require.Contains(t, stderr, "Warning: ")
	require.Contains(t, stderr, "config")
}

func TestRoot_WarnsWhenConfigWeakensTLS(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		server string
		warn   bool
	}{
		{"insecure to a remote host", "insecure", "api.example.test:443", true},
		{"plaintext to a remote host", "plaintext", "api.example.test:80", true},
		{"plaintext to localhost", "plaintext", "localhost:8080", false},
		{"insecure to a loopback IP", "insecure", "127.0.0.1:8443", false},
		{"insecure to IPv6 loopback", "insecure", "[::1]:8443", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, map[string]string{tc.key: "true", "server": tc.server})
			_, _, stderr, code := runRoot(t, []string{"--config-dir", dir, "version"}, nil)
			require.Equal(t, -1, code)
			if tc.warn {
				require.Contains(t, stderr, "config.json sets "+tc.key+"=true")
				require.Contains(t, stderr, "admiral config unset "+tc.key)
			} else {
				require.Empty(t, stderr)
			}
		})
	}

	t.Run("the flag is not warned about", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, map[string]string{"server": "api.example.test:443"})
		_, _, stderr, code := runRoot(t, []string{"--config-dir", dir, "--insecure", "version"}, nil)
		require.Equal(t, -1, code)
		require.Empty(t, stderr)
	})
}

// --no-input reaches prompts through the Streams on the context, not the
// environment: after the run, the variable is untouched.
func TestRoot_NoInputDoesNotTouchEnvironment(t *testing.T) {
	t.Setenv("ADMIRAL_NO_INPUT", "")
	t.Setenv("ADMIRAL_FORCE_INTERACTIVE", "1")

	probe := func(args ...string) *iostreams.Streams {
		root, err := newRootCmd(version.GetVersion(), func(int) {})
		require.NoError(t, err)
		var seen *iostreams.Streams
		root.cmd.AddCommand(&cobra.Command{
			Use: "probe",
			RunE: func(cmd *cobra.Command, _ []string) error {
				seen = iostreams.FromCommand(cmd)
				return nil
			},
		})
		root.cmd.SetOut(io.Discard)
		root.cmd.SetErr(io.Discard)
		root.Execute(append([]string{"--config-dir", t.TempDir()}, append(args, "probe")...))
		require.NotNil(t, seen)
		return seen
	}

	require.False(t, probe("--no-input").Interactive())
	require.Empty(t, os.Getenv("ADMIRAL_NO_INPUT"))
	require.True(t, probe().Interactive(), "without the flag the forced-interactive session prompts")
}
