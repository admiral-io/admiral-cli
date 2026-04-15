package source

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	var srcID string

	cmd := &cobra.Command{
		Use:   "get [source]",
		Short: "Get source details",
		Long: `Get detailed information about a source.

The source can be provided as a positional argument (name) or looked up by UUID with --id.`,
		Example: `  # Get source by name
  admiral source get acme-infra

  # Get source by UUID
  admiral source get --id 550e8400-e29b-41d4-a716-446655440000`,
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

			resp, err := c.Source().GetSource(cmd.Context(), &sourcev1.GetSourceRequest{
				SourceId: id,
			})
			if err != nil {
				return err
			}
			s := resp.Source

			credID := "-"
			if s.CredentialId != nil {
				credID = *s.CredentialId
			}

			details := []output.Detail{
				{Key: "ID", Value: s.Id},
				{Key: "Name", Value: s.Name},
				{Key: "Type", Value: formatSourceType(s.Type)},
				{Key: "URL", Value: s.Url},
				{Key: "Credential ID", Value: credID},
				{Key: "Catalog", Value: fmt.Sprintf("%t", s.Catalog)},
				{Key: "Description", Value: s.Description},
				{Key: "Labels", Value: output.FormatLabels(s.Labels)},
				{Key: "Last Test", Value: formatTestStatus(s.LastTestStatus)},
				{Key: "Last Test Error", Value: s.LastTestError},
				{Key: "Last Tested", Value: output.FormatTimestamp(s.LastTestedAt)},
				{Key: "Last Synced", Value: output.FormatTimestamp(s.LastSyncedAt)},
				{Key: "Created", Value: output.FormatTimestamp(s.CreatedAt)},
				{Key: "Created By", Value: s.CreatedBy.GetId()},
				{Key: "Updated", Value: output.FormatTimestamp(s.UpdatedAt)},
				{Key: "Age", Value: output.FormatAge(s.CreatedAt)},
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintDetail(resp, []output.Section{{Details: details}})
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	return cmd
}
