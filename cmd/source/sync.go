package source

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newSyncCmd(opts *client.Options) *cobra.Command {
	var srcID string

	cmd := &cobra.Command{
		Use:   "sync [source]",
		Short: "Fetch and refresh source metadata",
		Long:  `Materialize the source's content (clone / download / pull) and update last_synced_at.`,
		Args:  cobra.MaximumNArgs(1),
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
			resp, err := c.Source().SyncSource(cmd.Context(), &sourcev1.SyncSourceRequest{SourceId: id})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				printSourceRow(w, resp.Source)
			})
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	return cmd
}