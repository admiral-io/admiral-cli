package cmd

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

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
	root := newRootCmd(version.GetVersion(), os.Exit)
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
