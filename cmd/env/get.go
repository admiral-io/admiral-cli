package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
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
		Example: `  admiral env get staging --app billing
  admiral env get <uuid>`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), appName, args[0])
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
