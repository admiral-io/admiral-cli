package source

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a source",
		Long: `Delete a source.

The source is given by name or ID.`,
		Example: `  # Delete a source by name (prompts to confirm)
  admiral source delete acme-infra

  # Skip the prompt
  admiral source delete acme-infra --force`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}

			display := args[0]
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Delete source %s", display)); err != nil {
				return err
			}

			if _, err := c.Source().DeleteSource(cmd.Context(), &sourcev1.DeleteSourceRequest{
				SourceId: id,
			}); err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "source", display, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
