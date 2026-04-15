package source

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var srcID string

	cmd := &cobra.Command{
		Use:   "delete [source]",
		Short: "Delete a source",
		Long: `Delete a source.

The source can be provided as a positional argument (name) or looked up by UUID with --id.`,
		Example: `  # Delete a source by name
  admiral source delete acme-infra

  # Delete by UUID
  admiral source delete --id 550e8400-e29b-41d4-a716-446655440000`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && srcID == "" {
				_ = cmd.Help()
				return fmt.Errorf("source name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveSourceID(cmd.Context(), c.Source(), nameArg, srcID)
			if err != nil {
				return err
			}
			if _, err := c.Source().DeleteSource(cmd.Context(), &sourcev1.DeleteSourceRequest{
				SourceId: id,
			}); err != nil {
				return err
			}
			output.Writef(cmd.OutOrStdout(), "Source %s deleted\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	return cmd
}
