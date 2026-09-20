package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		appName   string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list [app]",
		Short: "List environments",
		Long:  `List the environments of an application, named as the argument or with --app.`,
		Example: `  # Environments of an application
  admiral env list billing

  # Names only, for scripting
  admiral env list billing -o name`,
		Args:              flags.MaximumNArgs(1),
		ValidArgsFunction: complete.First(complete.Apps(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := flags.AppTarget(cmd, appName, args)
			if err != nil {
				return err
			}
			// Checked before the client is built: dialing first would
			// report a missing credential for what is a missing argument.
			if app == "" {
				return cmderr.UsageHint("Name the application: 'admiral env list billing'.",
					"no application specified")
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

			filter, err := filter.Eq("application_id", resolvedAppID)
			if err != nil {
				return err
			}
			resp, err := c.Environment().ListEnvironments(cmd.Context(), &environmentv1.ListEnvironmentsRequest{
				Filter:    filter,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "environments",
				Items:         output.Messages(resp.Environments),
				Name:          func(i int) string { return resp.Environments[i].Name },
				NextPageToken: resp.NextPageToken,
			}, envTable.Render(p, resp.Environments...))
		},
	}

	flags.App(cmd, &appName, opts)
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")

	return cmd
}
