package changeset

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		status string
		paging flags.PagingOptions
	)

	cmd := &cobra.Command{
		Use:   "list <app>/<env>",
		Short: "List an environment's changesets",
		Example: `  admiral changeset list shop/prod

  # Open ones only, IDs for scripting
  admiral changeset list shop/prod --status open -o name`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			envTarget := args[0]
			app, env, err := flags.EnvTarget(cmd, "", envTarget)
			if err != nil {
				return err
			}
			// Checked before the client is built: dialing first would report
			// a missing credential for what is a missing argument.
			if app == "" && !resolve.IsID("env", env) {
				return cmderr.UsageHint("Give the environment as app/env: 'admiral changeset list shop/prod'.",
					"no application specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, env)
			if err != nil {
				return err
			}
			var byStatus string
			if status != "" {
				if byStatus, err = filter.Eq("status", "STATUS_"+strings.ToUpper(status)); err != nil {
					return err
				}
			}
			css, next, err := flags.Pages(paging, func(token string) ([]*changesetv1.ChangeSet, string, error) {
				resp, err := c.ChangeSet().ListChangeSets(cmd.Context(), &changesetv1.ListChangeSetsRequest{
					EnvironmentId: envID,
					Filter:        byStatus,
					PageSize:      paging.PageSize,
					PageToken:     token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.ChangeSets, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "changesets",
				Scope:         envTarget,
				Items:         output.Messages(css),
				Name:          func(i int) string { return css[i].Id },
				NextPageToken: next,
			}, changeSetTable.Render(p, css...))
		},
	}

	flags.Enum(cmd, &status, "status", "", "only changesets in this status", "open", "applied", "closed")
	flags.Paging(cmd, &paging)

	return cmd
}
