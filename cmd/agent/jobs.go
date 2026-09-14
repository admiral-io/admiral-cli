package agent

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newJobsCmd(opts *client.Options) *cobra.Command {
	var (
		statusFlag string
		typeFlag   string
		runID      string
		pageSize   int32
		pageToken  string
	)

	cmd := &cobra.Command{
		Use:   "jobs <name>",
		Short: "List jobs assigned to an agent",
		Long: `List the jobs assigned to an agent, ordered from newest to oldest. Use --status
or --type to narrow the result, and --run to scope to a single run.`,
		Example: `  # All jobs on an agent
  admiral agent jobs prod-agent

  # Only currently-running jobs
  admiral agent jobs prod-agent --status running

  # Failed plan jobs
  admiral agent jobs prod-agent --status failed --type plan

  # Jobs for a specific run
  admiral agent jobs prod-agent --run <uuid>`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Agent(cmd.Context(), c.Agent(), args[0])
			if err != nil {
				return err
			}

			filter, err := buildJobsFilter(statusFlag, typeFlag, runID)
			if err != nil {
				return err
			}

			resp, err := c.Agent().ListAgentJobs(cmd.Context(), &agentv1.ListAgentJobsRequest{
				AgentId:   id,
				Filter:    filter,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "jobs",
				Items:         output.Messages(resp.Jobs),
				Name:          func(i int) string { return resp.Jobs[i].Id },
				NextPageToken: resp.NextPageToken,
			}, jobTable.Render(p, resp.Jobs...))
		},
	}

	flags.Enum(cmd, &statusFlag, "status", "", "filter by job status", "pending", "assigned", "running", "succeeded", "failed", "canceled")
	flags.Enum(cmd, &typeFlag, "type", "", "filter by job type", "plan", "apply", "destroy-plan", "destroy-apply")
	cmd.Flags().StringVar(&runID, "run", "", "filter by run UUID")
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	return cmd
}

func buildJobsFilter(statusFlag, typeFlag, runID string) (string, error) {
	var parts []string

	if statusFlag != "" {
		pred, err := filter.Eq("status", jobStatusFromString[statusFlag].String())
		if err != nil {
			return "", err
		}
		parts = append(parts, pred)
	}

	if typeFlag != "" {
		pred, err := filter.Eq("job_type", jobTypeFromString[typeFlag].String())
		if err != nil {
			return "", err
		}
		parts = append(parts, pred)
	}

	if runID != "" {
		pred, err := filter.Eq("run_id", runID)
		if err != nil {
			return "", err
		}
		parts = append(parts, pred)
	}

	return filter.And(parts...), nil
}

func jobDuration(j *agentv1.Job) string {
	if j.StartedAt == nil {
		return "-"
	}
	end := j.CompletedAt
	if end == nil {
		end = timestamppb.Now()
	}
	d := end.AsTime().Sub(j.StartedAt.AsTime())
	if d < 0 {
		return "-"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
