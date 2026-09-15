package env

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
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
		Example: `  admiral env delete staging --app billing
  admiral env delete <uuid>
  admiral env delete staging --app billing --force`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			display := args[0]
			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), appName, args[0])
			if err != nil {
				return err
			}

			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Delete environment %s/%s", appName, display)); err != nil {
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
