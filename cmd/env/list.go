package env

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		appName string
		paging  flags.PagingOptions
	)

	cmd := &cobra.Command{
		Use:   "list [app]",
		Short: "List environments",
		Long:  `List the environments of a project, named as the argument or with --project.`,
		Example: `  # Environments of a project
  admiral env list billing

  # Names only, for scripting
  admiral env list billing -o name`,
		Args:              flags.MaximumNArgs(1),
		ValidArgsFunction: complete.First(complete.Projects(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := flags.ProjectTarget(cmd, appName, args)
			if err != nil {
				return err
			}
			// Checked before the client is built: dialing first would
			// report a missing credential for what is a missing argument.
			if app == "" {
				return cmderr.UsageHint("Name the project: 'admiral env list billing'.",
					"no project specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resolvedAppID, err := resolve.Project(cmd.Context(), c.Project(), app)
			if err != nil {
				return err
			}

			envs, next, err := flags.Pages(paging, func(token string) ([]*environmentv1.Environment, string, error) {
				resp, err := c.Environment().ListEnvironments(cmd.Context(), &environmentv1.ListEnvironmentsRequest{
					ProjectId: &resolvedAppID,
					PageSize:  paging.PageSize,
					PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Environments, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "environments",
				Items:         output.Messages(envs),
				Name:          func(i int) string { return envs[i].Name },
				NextPageToken: next,
			}, envTable.Render(p, envs...))
		},
	}

	flags.Project(cmd, &appName, opts)
	flags.Paging(cmd, &paging)

	return cmd
}
