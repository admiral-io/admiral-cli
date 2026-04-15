package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	var appID string

	cmd := &cobra.Command{
		Use:   "get [app]",
		Short: "Get application details",
		Long: `Get detailed information about an application.

The app can be provided as a positional argument (name) or looked up by UUID with --id.`,
		Example: `  # Get app by name
  admiral app get billing-api

  # Get app by UUID
  admiral app get --id 550e8400-e29b-41d4-a716-446655440000`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var appArg string
			if len(args) == 1 {
				appArg = args[0]
			}
			if appArg == "" && appID == "" {
				_ = cmd.Help()
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				return fmt.Errorf("app name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			id, err := util.ResolveAppID(cmd.Context(), c.Application(), appArg, appID)
			if err != nil {
				return err
			}

			resp, err := c.Application().GetApplication(cmd.Context(), &applicationv1.GetApplicationRequest{
				ApplicationId: id,
			})
			if err != nil {
				return err
			}

			app := resp.Application
			p := output.NewPrinter(opts.OutputFormat)

			sections := []output.Section{
				{
					Details: []output.Detail{
						{Key: "ID", Value: app.Id},
						{Key: "Name", Value: app.Name},
						{Key: "Description", Value: app.Description},
						{Key: "Labels", Value: output.FormatLabels(app.Labels)},
						{Key: "Created", Value: output.FormatTimestamp(app.CreatedAt)},
						{Key: "Updated", Value: output.FormatTimestamp(app.UpdatedAt)},
						{Key: "Age", Value: output.FormatAge(app.CreatedAt)},
					},
				},
			}

			return p.PrintDetail(resp, sections)
		},
	}

	cmd.Flags().StringVar(&appID, "id", "", "application ID (UUID)")

	return cmd
}
