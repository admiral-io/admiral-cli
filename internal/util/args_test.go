package util

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestExactArgs_Passes(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Args: ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	cmd.SetArgs([]string{"arg1"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestExactArgs_TooFew(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{
		Use:   "get <cluster-id>",
		Short: "Get a cluster",
		Args:  ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 1 arg(s), received 0")

	// Help should have been printed to stdout
	require.Contains(t, stdout.String(), "Get a cluster")
	require.Contains(t, stdout.String(), "get <cluster-id>")
}

func TestExactArgs_TooMany(t *testing.T) {
	var stdout bytes.Buffer
	cmd := &cobra.Command{
		Use:   "get <cluster-id>",
		Short: "Get a cluster",
		Args:  ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"a", "b"})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 1 arg(s), received 2")
	require.Contains(t, stdout.String(), "Get a cluster")
}

func TestExactArgs_Two(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "get <cluster-id> <token-id>",
		Args: ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	cmd.SetArgs([]string{"cluster1", "token1"})
	require.NoError(t, cmd.Execute())

	cmd.SetArgs([]string{"cluster1"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 2 arg(s), received 1")
}

func TestExactArgs_Zero(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "list",
		Args: ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	cmd.SetArgs([]string{"extra"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 0 arg(s), received 1")
}
