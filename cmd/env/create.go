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

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		appName     string
		description string
		labelStrs   []string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an environment",
		Example: `  # Create an environment
  admiral env create billing/staging

  # Create with a description and labels
  admiral env create billing/prod --description "US East production" --label tier=1

  # By name, scoped with --app
  admiral env create staging --app billing`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.NewEnv(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resolvedAppID, err := resolve.App(cmd.Context(), c.Application(), app)
			if err != nil {
				return err
			}

			req := &environmentv1.CreateEnvironmentRequest{
				ApplicationId: resolvedAppID,
				Name:          name,
				Description:   description,
				Labels:        labels,
			}

			resp, err := c.Environment().CreateEnvironment(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Environment, resp.Environment.Name, envTable.Render(p, resp.Environment))
		},
	}

	flags.App(cmd, &appName, opts)
	cmd.Flags().StringVar(&description, "description", "", "environment description")
	flags.Label(cmd, &labelStrs, "label to attach (key=value, repeatable)")

	return cmd
}
