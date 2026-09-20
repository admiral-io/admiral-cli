package run

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		appName   string
		envName   string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list [app|app/env]",
		Short: "List runs",
		Long:  `List runs, scoped to an application or to one of its environments.`,
		Example: `  # Runs in one environment
  admiral run list billing/staging

  # Every environment of an application
  admiral run list billing

  # By name, scoped with --app
  admiral run list --app billing --env staging`,
		Args:              flags.MaximumNArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := flags.ScopeTarget(cmd, appName, envName, args)
			if err != nil {
				return err
			}
			// Checked before the client is built: dialing first would
			// report a missing credential for what is a missing argument.
			if app == "" {
				return cmderr.UsageHint("Name the scope: 'admiral run list billing' or 'admiral run list billing/staging'.",
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

			byApp, err := filter.Eq("application_id", resolvedAppID)
			if err != nil {
				return err
			}
			f := byApp
			if env != "" {
				resolvedEnvID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, env)
				if err != nil {
					return err
				}
				byEnv, err := filter.Eq("environment_id", resolvedEnvID)
				if err != nil {
					return err
				}
				f = filter.And(byApp, byEnv)
			}

			resp, err := c.Run().ListRuns(cmd.Context(), &runv1.ListRunsRequest{
				Filter:    f,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "runs",
				Items:         output.Messages(resp.Runs),
				Name:          func(i int) string { return RunID(resp.Runs[i]) },
				NextPageToken: resp.NextPageToken,
			}, RunTable.Render(p, resp.Runs...))
		},
	}

	flags.App(cmd, &appName, opts)
	flags.Env(cmd, &envName, opts)
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")

	return cmd
}
