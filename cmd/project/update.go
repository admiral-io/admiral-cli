package project

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		newName     string
		labelStrs   []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a project",
		Long: `Update a project's name, description, or labels. Only the fields you
pass are changed.`,
		Example: `  # Change the description
  admiral project update billing-api --description "Billing and invoicing"

  # Add or update a label
  admiral project update billing-api --label team=payments

  # Remove a label
  admiral project update billing-api --label team-

  # Set one label and remove another in the same call
  admiral project update billing-api --label tier=prod --label legacy-

  # Rename
  admiral project update billing-api --name billing`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Projects(opts)),
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

			id, err := resolve.Project(cmd.Context(), c.Project(), args[0])
			if err != nil {
				return err
			}

			// Read-modify-write so untouched fields keep their values.
			current, err := c.Project().GetProject(cmd.Context(), &projectv1.GetProjectRequest{
				ProjectId: id,
			})
			if err != nil {
				return err
			}
			project := current.Project

			if cmd.Flags().Changed("name") {
				project.Name = newName
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(project.Labels, labelStrs)
				if err != nil {
					return err
				}
				project.Labels = labels
			}
			if cmd.Flags().Changed("description") {
				project.Description = description
			}

			resp, err := c.Project().UpdateProject(cmd.Context(), &projectv1.UpdateProjectRequest{
				Project:    project,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Project, resp.Project.Name, projectTable.Render(p, resp.Project))
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new project name")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "new description")

	return cmd
}
