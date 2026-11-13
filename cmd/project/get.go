package project

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a project",
		Long: `Print the one-line summary of a project. Use 'describe' for the full
view and '-o json' for the raw record.`,
		Example: `  # Get a project by name
  admiral project get billing-api

  # Get a project by ID
  admiral project get prj-7q2k9m4x8p1c

  # Raw record as JSON
  admiral project get billing-api -o json`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Projects(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			id, err := resolve.Project(cmd.Context(), c.Project(), args[0])
			if err != nil {
				return err
			}

			resp, err := c.Project().GetProject(cmd.Context(), &projectv1.GetProjectRequest{
				ProjectId: id,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Project, resp.Project.Name, projectTable.Render(p, resp.Project))
		},
	}

	return cmd
}
