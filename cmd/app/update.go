package app

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		appID       string
		newName     string
		labelStrs   []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "update [app]",
		Short: "Update an application",
		Long: `Update an existing application.

The app can be provided as a positional argument (name) or looked up by UUID with --id.`,
		Example: `  # Update labels by name (default)
  admiral app update billing-api --label team=payments

  # Update by UUID
  admiral app update --id 550e8400-e29b-41d4-a716-446655440000 --label team=payments

  # Update description
  admiral app update billing-api --description "New description"`,
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

			var paths []string
			if cmd.Flags().Changed("name") {
				paths = append(paths, "name")
			}
			if cmd.Flags().Changed("label") {
				paths = append(paths, "labels")
			}
			if cmd.Flags().Changed("description") {
				paths = append(paths, "description")
			}
			if len(paths) == 0 {
				return fmt.Errorf("at least one of --name, --label, or --description must be specified")
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

			// Read-modify-write so untouched fields keep their values.
			current, err := c.Application().GetApplication(cmd.Context(), &applicationv1.GetApplicationRequest{
				ApplicationId: id,
			})
			if err != nil {
				return err
			}
			application := current.Application

			if cmd.Flags().Changed("name") {
				application.Name = newName
			}
			if cmd.Flags().Changed("label") {
				labels, err := util.ParseLabels(labelStrs)
				if err != nil {
					return err
				}
				application.Labels = labels
			}
			if cmd.Flags().Changed("description") {
				application.Description = description
			}

			resp, err := c.Application().UpdateApplication(cmd.Context(), &applicationv1.UpdateApplicationRequest{
				Application: application,
				UpdateMask:  &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				app := resp.Application
				output.Writeln(w, "NAME\tDESCRIPTION\tAGE")
				output.Writef(w, "%s\t%s\t%s\n",
					app.Name,
					app.Description,
					output.FormatAge(app.CreatedAt),
				)
			})
		},
	}

	cmd.Flags().StringVar(&appID, "id", "", "application ID (UUID)")
	cmd.Flags().StringVar(&newName, "name", "", "new application name")
	util.AddLabelFlag(cmd, &labelStrs, "label to set (key=value, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "application description")

	return cmd
}
