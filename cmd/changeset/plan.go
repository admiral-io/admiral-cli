package changeset

import (
	"github.com/spf13/cobra"

	runcmd "go.admiral.io/cli/cmd/run"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newPlanCmd(opts *client.Options) *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "plan <changeset-id>",
		Short: "Plan the change set",
		Long: `Create a run that plans the change set's entries and variables.
If a prior plan exists for the same change set, it is superseded automatically.
The latest plan is the one applied when you run 'changeset apply'.`,
		Example: `  admiral changeset plan cs-1
  admiral changeset plan cs-1 -m "rebase against new bootstrap"`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			// CreateRun still requires application_id and environment_id explicitly
			// (no server-side derivation when change_set_id is present), so we
			// fetch the change set to read those fields. The change_set_id itself
			// is forwarded verbatim -- the server accepts both UUID and cs-<suffix>.
			cs, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: args[0]})
			if err != nil {
				return err
			}
			resp, err := c.Run().CreateRun(cmd.Context(), &runv1.CreateRunRequest{
				ApplicationId: cs.ChangeSet.ApplicationId,
				EnvironmentId: cs.ChangeSet.EnvironmentId,
				ChangeSetId:   args[0],
				Message:       message,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Run, runcmd.RunID(resp.Run), runcmd.RunTable.Render(p, resp.Run))
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "run message")
	return cmd
}
