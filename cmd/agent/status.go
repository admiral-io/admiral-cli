package agent

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newStatusCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show an agent's live status",
		Long: `Show the server's current snapshot of an agent: health, capacity,
and in-flight job phases.`,
		Example: `  # Current status of an agent by name
  admiral agent status prod-agent

  # By UUID
  admiral agent status <uuid>`,
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

			resp, err := c.Agent().GetAgentStatus(cmd.Context(), &agentv1.GetAgentStatusRequest{
				AgentId: id,
			})
			if err != nil {
				return err
			}

			infra := resp.GetTerraform()
			workload := resp.GetKubernetes()

			d := output.NewDescribe()
			d.Field("Health", output.FormatEnum(resp.HealthStatus))
			d.Field("Reported", formatReportedAt(resp))
			if infra == nil && workload == nil {
				d.Field("Note", "agent has not reported yet")
			}
			if infra != nil {
				d.Field("Version", infra.Version)
				d.Field("Capacity", fmt.Sprintf("%d / %d jobs", infra.ActiveJobs, infra.MaxConcurrentJobs))
				if len(infra.ActiveJobDetails) > 0 {
					rows := make([][]string, 0, len(infra.ActiveJobDetails))
					for _, aj := range infra.ActiveJobDetails {
						rows = append(rows, []string{aj.JobId, output.FormatEnum(aj.Phase), output.FormatAge(aj.StartedAt)})
					}
					d.Table("Active Jobs", []string{"Job", "Phase", "Age"}, rows)
				}
			}
			if workload != nil {
				d.Field("K8s Version", workload.K8SVersion)
				d.Field("Nodes", fmt.Sprintf("%d / %d ready", workload.NodesReady, workload.NodeCount))
				d.Field("Workloads", fmt.Sprintf("%d total, %d healthy", workload.WorkloadsTotal, workload.WorkloadsHealthy))
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintStatus(resp, d)
		},
	}

	return cmd
}

func formatReportedAt(resp *agentv1.GetAgentStatusResponse) string {
	if resp == nil || resp.ReportedAt == nil {
		return "<never>"
	}
	return fmt.Sprintf("%s (%s ago)", output.FormatTimestamp(resp.ReportedAt), output.FormatAge(resp.ReportedAt))
}
