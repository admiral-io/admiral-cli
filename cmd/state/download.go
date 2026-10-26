package state

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
)

func newDownloadCmd(opts *client.Options) *cobra.Command {
	var (
		out   string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "download <version-id>",
		Short: "Download one state version",
		Long: `Download one version of a component's state, to stdout or to a file.

State can hold secrets: a file is written readable by you alone, and an
existing file is kept unless --force.`,
		Example: `  admiral state download stv-3k9x2m1qa8z0 -f network.tfstate

  # Roll back: push the earlier version over the current one
  terraform state push -force network.tfstate`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := client.NewStateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			body, err := sc.DownloadStateVersion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			defer body.Close() //nolint:errcheck
			if out == "" {
				_, err = io.Copy(cmd.OutOrStdout(), body)
				return err
			}
			if err := writeState(out, force, body); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output-file", "f", "", "write to this file instead of stdout")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite the file if it exists")
	return cmd
}

// writeState writes a state to a file only its owner reads, keeping one that
// exists unless force.
func writeState(path string, force bool, body io.Reader) error {
	mode := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, mode, 0o600)
	if errors.Is(err, os.ErrExist) {
		return cmderr.Usage("%s exists; pass --force to overwrite it", path)
	}
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
