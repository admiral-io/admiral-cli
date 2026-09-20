package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		appName string
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an environment",
		Example: `  # By path
  admiral env delete billing/staging

  # By name, scoped with --app
  admiral env delete staging --app billing

  # By ID
  admiral env delete <uuid>

  # Skip the confirmation prompt
  admiral env delete billing/staging --force`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}
			// Before any network use: a run that cannot confirm and did not
			// pass --force fails here, in milliseconds, with the usage error.
			if err := input.RequireInteractiveOrForce(cmd, force); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			display := name
			if app != "" {
				display = app + "/" + name
			}
			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, name)
			if err != nil {
				return err
			}

			if err := input.Confirm(cmd, force, "Delete environment "+display); err != nil {
				return err
			}

			if _, err := c.Environment().DeleteEnvironment(cmd.Context(), &environmentv1.DeleteEnvironmentRequest{
				EnvironmentId: envID,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "environment", display, "deleted")
			return nil
		},
	}

	flags.App(cmd, &appName, opts)
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
