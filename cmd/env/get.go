package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {
	var (
		appName string
	)

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get an environment",
		Example: `  # By path
  admiral env get billing/staging

  # By name, scoped with --app
  admiral env get staging --app billing

  # By ID
  admiral env get <uuid>`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, name)
			if err != nil {
				return err
			}

			resp, err := c.Environment().GetEnvironment(cmd.Context(), &environmentv1.GetEnvironmentRequest{
				EnvironmentId: envID,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Environment, resp.Environment.Name, envTable.Render(p, resp.Environment))
		},
	}

	flags.App(cmd, &appName, opts)

	return cmd
}
