package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
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
  admiral env create staging --app billing

  # Create with a description and labels
  admiral env create prod --app billing --description "US East production" --label tier=1`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resolvedAppID, err := resolve.App(cmd.Context(), c.Application(), appName)
			if err != nil {
				return err
			}

			req := &environmentv1.CreateEnvironmentRequest{
				ApplicationId: resolvedAppID,
				Name:          args[0],
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
