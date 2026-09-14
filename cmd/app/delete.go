package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an application",
		Long: `Delete an application and everything in it: its environments, components,
and runs. You must type the application name to confirm, or pass --force to
skip the prompt.`,
		Example: `  # Delete an application by name (prompts for the name to confirm)
  admiral app delete billing-api

  # Skip the prompt
  admiral app delete billing-api --force`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Apps(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			id, err := resolve.App(cmd.Context(), c.Application(), args[0])
			if err != nil {
				return err
			}

			name := args[0]
			if name == "" {
				resp, err := c.Application().GetApplication(cmd.Context(), &applicationv1.GetApplicationRequest{ApplicationId: id})
				if err != nil {
					return err
				}
				name = resp.Application.Name
			}

			prompt := fmt.Sprintf("Deleting %s is not reversible and removes all of its environments, components and runs.", name)
			if err := input.ConfirmName(cmd, force, prompt, name); err != nil {
				return err
			}

			if _, err := c.Application().DeleteApplication(cmd.Context(), &applicationv1.DeleteApplicationRequest{
				ApplicationId: id,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "application", name, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
