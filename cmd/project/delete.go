package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		yes bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a project",
		Long: `Delete a project that has no environments left. Admiral refuses to
delete one that still has any: delete each first with
'admiral env delete <project>/<environment>'. You must type the project
name to confirm, or pass --yes to skip the prompt.`,
		Example: `  # Delete a project by name (prompts for the name to confirm)
  admiral project delete billing-api

  # Skip the prompt
  admiral project delete billing-api --yes`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Projects(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Before any network use: a run that cannot confirm and did not
			// pass --yes fails here, in milliseconds, with the usage error.
			if err := input.RequireInteractiveOrYes(cmd, yes); err != nil {
				return err
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

			// The confirmation is typed by name. Given an ID, look the name
			// up so the user is not asked to type an id.
			name := args[0]
			if resolve.IsID("prj", name) {
				resp, err := c.Project().GetProject(cmd.Context(), &projectv1.GetProjectRequest{ProjectId: id})
				if err != nil {
					return err
				}
				name = resp.Project.Name
			}

			prompt := fmt.Sprintf("Deleting %s is not reversible.", name)
			if err := input.ConfirmName(cmd, yes, prompt, name); err != nil {
				return err
			}

			if _, err := c.Project().DeleteProject(cmd.Context(), &projectv1.DeleteProjectRequest{
				ProjectId: id,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "project", name, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")

	return cmd
}
