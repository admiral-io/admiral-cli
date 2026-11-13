package project

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs   []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project",
		Long: `Create a project with the given name. Add environments to it with
'admiral env create'.`,
		Example: `  # Create a project
  admiral project create billing-api

  # Create with a description and labels
  admiral project create billing-api --description "Handles billing" --label team=platform`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			req := &projectv1.CreateProjectRequest{Project: &projectv1.Project{
				Name:        args[0],
				Description: description,
				Labels:      labels,
			}}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.Project().CreateProject(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Project, resp.Project.Name, projectTable.Render(p, resp.Project))
		},
	}

	flags.Label(cmd, &labelStrs, "label to attach (key=value, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "project description")

	return cmd
}
