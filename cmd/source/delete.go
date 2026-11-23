package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a source",
		Long: `Delete a source. What was imported through it stays published, with
its provenance; nothing new comes through it.`,
		Example: `  admiral source delete argo-helm

  admiral source delete argo-helm --yes`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Sources(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}
			if err := input.Confirm(cmd, yes, "Delete source "+src.Name); err != nil {
				return err
			}
			if _, err := c.Source().DeleteSource(cmd.Context(), &sourcev1.DeleteSourceRequest{SourceId: src.Id}); err != nil {
				return err
			}
			output.Confirmed(cmd.ErrOrStderr(), "source", src.Name, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
