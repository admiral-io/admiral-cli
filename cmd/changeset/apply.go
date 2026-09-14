package changeset

import (
	"fmt"

	"github.com/spf13/cobra"

	runcmd "go.admiral.io/cli/cmd/run"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newApplyCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <changeset-id>",
		Short: "Apply the change set's planned run",
		Long: `Apply the latest plan for a change set. The change set must have a run in
PLANNED status (run 'changeset plan' first). Operators interact with the
change set; the underlying run id is resolved server-side.`,
		Example: `  admiral changeset apply cs-1`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			// The ListRuns filter parser does not yet resolve display IDs (Phase
			// 1.B explicitly defers that); change_set_id in a filter expression
			// must be a UUID. Resolve once here and use the canonical Id.
			cs, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: args[0]})
			if err != nil {
				return err
			}

			// Find the change set's most recent run; the server's ApplyRun
			// arbitrates by revision status (rejects when none are PLANNED).
			// Under Option B auto-continue the run may show PLANNED with
			// downstream DEFERRED revisions still gated by an upstream apply.
			byChangeSet, err := filter.Eq("change_set_id", cs.ChangeSet.Id)
			if err != nil {
				return err
			}
			runs, err := c.Run().ListRuns(cmd.Context(), &runv1.ListRunsRequest{
				Filter:   byChangeSet,
				PageSize: 1,
			})
			if err != nil {
				return err
			}
			if len(runs.Runs) == 0 {
				return fmt.Errorf("change set %s has no run; run 'changeset plan %s' first", args[0], args[0])
			}
			runID := runs.Runs[0].Id

			resp, err := c.Run().ApplyRun(cmd.Context(), &runv1.ApplyRunRequest{
				RunId: runID,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Run, runcmd.RunID(resp.Run), runcmd.RunTable.Render(p, resp.Run))
		},
	}
	return cmd
}
