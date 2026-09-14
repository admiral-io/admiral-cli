package run

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <run-id>",
		Short:   "Show everything about a run",
		Long:    "Render a run's identity, timing and every component revision with its outcome.\n\ndescribe is a human view. Use 'run get -o json' for the raw record.",
		Example: "  admiral run describe run-vnx81r",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			ctx := cmd.Context()

			runResp, err := c.Run().GetRun(ctx, &runv1.GetRunRequest{RunId: args[0]})
			if err != nil {
				return err
			}
			r := runResp.Run
			revResp, err := c.Run().ListRevisions(ctx, &runv1.ListRevisionsRequest{RunId: args[0]})
			if err != nil {
				return err
			}

			d := output.NewDescribe()
			d.Field("ID", RunID(r))
			d.Field("Application", orID(r.ApplicationName, r.ApplicationId))
			d.Field("Environment", orID(r.EnvironmentName, r.EnvironmentId))
			d.Field("Status", output.FormatEnum(r.Status))
			cs := orID(r.ChangeSetDisplayId, r.ChangeSetId)
			if r.ChangeSetTitle != "" {
				cs = fmt.Sprintf("%s (%s)", cs, r.ChangeSetTitle)
			}
			d.Field("Change Set", cs)
			d.Field("Message", r.Message)
			d.Field("Triggered By", output.FormatActor(r.TriggeredBy))
			if r.SourceRunId != "" {
				d.Field("Source Run", r.SourceRunId)
			}
			d.Field("Started", output.FormatDescribeTime(r.CreatedAt))
			d.Field("Finished", output.FormatDescribeTime(r.CompletedAt))
			if r.CompletedAt != nil {
				d.Field("Duration", output.FormatElapsed(r.CreatedAt, r.CompletedAt))
			}

			rows := make([][]string, 0, len(revResp.Revisions))
			hasTranscript := false
			for _, rev := range revResp.Revisions {
				phase := ""
				if n := len(rev.AvailablePhases); n > 0 {
					phase = output.FormatEnumKebab(rev.AvailablePhases[n-1])
					hasTranscript = true
				}
				rows = append(rows, []string{
					rev.ComponentName,
					output.FormatEnumKebab(rev.Kind),
					phase,
					output.FormatEnum(rev.Status),
					formatChangeSummary(rev.PlanSummary),
					strings.Join(rev.DependsOn, ", "),
					rev.ErrorMessage,
				})
			}
			d.Table("Revisions", []string{"Component", "Kind", "Phase", "Status", "Changes", "Depends On", "Error"}, rows)

			if hasTranscript {
				d.Hint("To see the transcript, run: admiral run logs " + RunID(r))
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral run get "+RunID(r))
		},
	}
	return cmd
}
