package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		appName     string
		envName     string
		title       string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a change set",
		Long:  `Create a new OPEN change set scoped to (application, environment). Returns the change set ID for use in subsequent entry / variable commands.`,
		Example: `  admiral changeset create --app billing --env staging --title "bump database"
  admiral changeset create --app billing --env prod --title "rotate region" --description "move to us-west-2"`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resolvedAppID, err := resolve.App(cmd.Context(), c.Application(), appName)
			if err != nil {
				return err
			}
			resolvedEnvID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), appName, envName)
			if err != nil {
				return err
			}

			resp, err := c.ChangeSet().CreateChangeSet(cmd.Context(), &changesetv1.CreateChangeSetRequest{
				ApplicationId: resolvedAppID,
				EnvironmentId: resolvedEnvID,
				Title:         title,
				Description:   description,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.ChangeSet, changeSetID(resp.ChangeSet), changeSetTable.Render(p, resp.ChangeSet))
		},
	}

	flags.App(cmd, &appName, opts)
	flags.Env(cmd, &envName)
	cmd.Flags().StringVar(&title, "title", "", "human-readable title")
	cmd.Flags().StringVar(&description, "description", "", "longer description")
	return cmd
}
