package app

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		appID   string
		confirm bool
	)

	cmd := &cobra.Command{
		Use:   "delete [app]",
		Short: "Delete an application",
		Long: `Delete an application.

The app can be provided as a positional argument (name) or looked up by UUID with --id.
Requires --confirm to prevent accidental deletion.`,
		Example: `  # Delete an application by name
  admctl app delete billing-api --confirm

  # Delete by UUID
  admctl app delete --id 550e8400-e29b-41d4-a716-446655440000 --confirm`,
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

			display := appArg
			if display == "" {
				display = appID
			}

			if !confirm {
				return fmt.Errorf("use --confirm to delete application %s", display)
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

			resp, err := c.Application().DeleteApplication(cmd.Context(), &applicationv1.DeleteApplicationRequest{
				ApplicationId: id,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				output.Writef(w, "Application %s deleted\n", display)
			})
		},
	}

	cmd.Flags().StringVar(&appID, "id", "", "application ID (UUID)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm deletion")

	return cmd
}
