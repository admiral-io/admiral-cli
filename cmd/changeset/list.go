package changeset

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var (
		appName   string
		envName   string
		statusF   string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List change sets",
		Long:  `List change sets, optionally scoped to an application/environment and filtered by status.`,
		Example: `  admiral changeset list --app billing
  admiral changeset list --env billing/staging --status OPEN`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, env, err := flags.EnvTarget(cmd, appName, envName)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			var clauses []string
			if app != "" {
				resolvedAppID, err := resolve.App(cmd.Context(), c.Application(), app)
				if err != nil {
					return err
				}
				clause, err := filter.Eq("application_id", resolvedAppID)
				if err != nil {
					return err
				}
				clauses = append(clauses, clause)
				if env != "" {
					resolvedEnvID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, env)
					if err != nil {
						return err
					}
					clause, err := filter.Eq("environment_id", resolvedEnvID)
					if err != nil {
						return err
					}
					clauses = append(clauses, clause)
				}
			}
			if statusF != "" {
				clause, err := filter.Eq("status", strings.ToUpper(statusF))
				if err != nil {
					return err
				}
				clauses = append(clauses, clause)
			}

			req := &changesetv1.ListChangeSetsRequest{
				Filter:    strings.Join(clauses, " AND "),
				PageSize:  pageSize,
				PageToken: pageToken,
			}

			resp, err := c.ChangeSet().ListChangeSets(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "change sets",
				Items:         output.Messages(resp.ChangeSets),
				Name:          func(i int) string { return changeSetID(resp.ChangeSets[i]) },
				NextPageToken: resp.NextPageToken,
			}, changeSetTable.Render(p, resp.ChangeSets...))
		},
	}

	flags.App(cmd, &appName, opts)
	flags.Env(cmd, &envName, opts)
	flags.Enum(cmd, &statusF, "status", "", "filter by status", "open", "deployed", "discarded")
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	return cmd
}
