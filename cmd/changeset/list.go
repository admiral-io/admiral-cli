package changeset

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		envTarget string
		status    string
		paging    flags.PagingOptions
	)

	cmd := &cobra.Command{
		Use:   "list --env <app>/<env>",
		Short: "List an environment's change sets",
		Example: `  admiral changeset list --env shop/prod

  # Open ones only, IDs for scripting
  admiral changeset list --env shop/prod --status open -o name`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if envTarget == "" {
				return cmderr.UsageHint("Pass --env as app/env: 'admiral changeset list --env shop/prod'.",
					"no environment specified")
			}
			app, env, err := flags.EnvTarget(cmd, "", envTarget)
			if err != nil {
				return err
			}
			// Checked before the client is built: dialing first would report
			// a missing credential for what is a missing argument.
			if app == "" && !resolve.IsID("env", env) {
				return cmderr.UsageHint("Pass --env as app/env: 'admiral changeset list --env shop/prod'.",
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
				Kind:          "change sets",
				Scope:         envTarget,
				Items:         output.Messages(css),
				Name:          func(i int) string { return css[i].Id },
				NextPageToken: next,
			}, changeSetTable.Render(p, css...))
		},
	}

	flags.Env(cmd, &envTarget, opts)
	flags.Enum(cmd, &status, "status", "", "only change sets in this status", "open", "applied", "closed")
	flags.Paging(cmd, &paging)

	return cmd
}
