package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newCopyCmd(opts *client.Options) *cobra.Command {
	var (
		envName     string
		title       string
		description string
	)

	cmd := &cobra.Command{
		Use:     "copy <id>",
		Short:   "Copy a change set to another environment",
		Long:    `Create a new OPEN change set in the target environment by cloning the source's entries and variable entries. Title/description default to the source's values when empty.`,
		Example: `  admiral changeset copy <source-id> --env prod`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: args[0]})
			if err != nil {
				return err
			}
			resolvedEnvID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), src.ChangeSet.ApplicationId, envName)
			if err != nil {
				return err
			}

			resp, err := c.ChangeSet().CopyChangeSet(cmd.Context(), &changesetv1.CopyChangeSetRequest{
				ChangeSetId:   args[0],
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
	flags.Env(cmd, &envName)
	cmd.Flags().StringVar(&title, "title", "", "title override (defaults to source title)")
	cmd.Flags().StringVar(&description, "description", "", "description override (defaults to source description)")
	return cmd
}
