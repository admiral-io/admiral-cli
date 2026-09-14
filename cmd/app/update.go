package app

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		newName     string
		labelStrs   []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an application",
		Long: `Update an application's name, description, or labels. Only the fields you
pass are changed.`,
		Example: `  # Change the description
  admiral app update billing-api --description "Billing and invoicing"

  # Add or update a label
  admiral app update billing-api --label team=payments

  # Remove a label
  admiral app update billing-api --label team-

  # Set one label and remove another in the same call
  admiral app update billing-api --label tier=prod --label legacy-

  # Rename
  admiral app update billing-api --name billing`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Apps(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return cmderr.Usage("at least one of --name, --label, or --description must be specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			id, err := resolve.App(cmd.Context(), c.Application(), args[0])
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
				labels, err := flags.ApplyLabelPatch(application.Labels, labelStrs)
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

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Application, resp.Application.Name, appTable.Render(p, resp.Application))
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new application name")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "new description")

	return cmd
}
