package app

import (
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		labelStrs   []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an application",
		Long:  `Create a new application with the given name.`,
		Example: `  # Create an application
  admiral app create billing-api

  # Create with labels
  admiral app create billing-api --label team=platform

  # Create with description
  admiral app create billing-api --description "Handles billing"`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := util.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			req := &applicationv1.CreateApplicationRequest{
				Name:   args[0],
				Labels: labels,
			}

			if cmd.Flags().Changed("description") {
				req.Description = &description
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.Application().CreateApplication(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				app := resp.Application
				output.Writeln(w, "NAME\tLABELS\tAGE")
				output.Writef(w, "%s\t%s\t%s\n",
					app.Name,
					output.FormatLabels(app.Labels),
					output.FormatAge(app.CreatedAt),
				)
			})
		},
	}

	util.AddLabelFlag(cmd, &labelStrs, "label to attach (key=value, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "application description")

	return cmd
}
