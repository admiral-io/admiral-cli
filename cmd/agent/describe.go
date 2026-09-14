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

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <name>",
		Short:   "Show everything about an agent",
		Long:    "Render an agent's identity, its last reported status, its tokens and its recent jobs.\n\ndescribe is a human view. Use 'agent get -o json' for the raw record.",
		Example: "  admiral agent describe gke-prod",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			ctx := cmd.Context()

			id, err := resolve.Agent(ctx, c.Agent(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Agent().GetAgent(ctx, &agentv1.GetAgentRequest{AgentId: id})
			if err != nil {
				return err
			}
			a := resp.Agent

			status, err := c.Agent().GetAgentStatus(ctx, &agentv1.GetAgentStatusRequest{AgentId: id})
			if err != nil {
				return fmt.Errorf("fetching status: %w", err)
			}
			tokens, err := c.Agent().ListApiKeys(ctx, &agentv1.ListApiKeysRequest{AgentId: id, PageSize: 50})
			if err != nil {
				return fmt.Errorf("listing tokens: %w", err)
			}
			jobs, err := c.Agent().ListAgentJobs(ctx, &agentv1.ListAgentJobsRequest{AgentId: id, PageSize: 5})
			if err != nil {
				return fmt.Errorf("listing jobs: %w", err)
			}

			d := output.NewDescribe()
			d.Field("Name", a.Name)
			d.Field("ID", a.Id)
			d.Field("Kind", output.FormatEnumKebab(a.Kind))
			d.Field("Description", a.Description)
			d.Fields("Labels", output.LabelLines(a.Labels))
			d.Field("Health", output.FormatEnum(status.HealthStatus))
			d.Field("Last Reported", formatReportedAt(status))
			d.Field("Cluster UID", a.ClusterUid)
			d.Field("Created", output.FormatDescribeTime(a.CreatedAt))
			d.Field("Created By", output.FormatActor(a.CreatedBy))

			if infra := status.GetTerraform(); infra != nil {
				d.Section("Status", func(b *output.Block) {
					b.Field("Version", infra.Version)
					b.Field("Capacity", fmt.Sprintf("%d / %d jobs", infra.ActiveJobs, infra.MaxConcurrentJobs))
					if len(infra.ActiveJobDetails) > 0 {
						rows := make([][]string, 0, len(infra.ActiveJobDetails))
						for _, aj := range infra.ActiveJobDetails {
							rows = append(rows, []string{aj.JobId, output.FormatEnum(aj.Phase), output.FormatAge(aj.StartedAt)})
						}
						b.Table([]string{"Job", "Phase", "Age"}, rows)
					}
				})
			}
			if wl := status.GetKubernetes(); wl != nil {
				d.Section("Status", func(b *output.Block) {
					b.Field("K8s Version", wl.K8SVersion)
					b.Field("Nodes", fmt.Sprintf("%d / %d ready", wl.NodesReady, wl.NodeCount))
					b.Field("Workloads", fmt.Sprintf("%d total, %d healthy", wl.WorkloadsTotal, wl.WorkloadsHealthy))
				})
			}

			tokenRows := make([][]string, 0, len(tokens.ApiKeys))
			for _, t := range tokens.ApiKeys {
				tokenRows = append(tokenRows, []string{t.Name, output.FormatEnum(t.Status), formatExpiry(t), output.FormatAge(t.CreatedAt)})
			}
			d.Table("Tokens", []string{"Name", "Status", "Expires", "Age"}, tokenRows)

			jobRows := make([][]string, 0, len(jobs.Jobs))
			for _, j := range jobs.Jobs {
				jobRows = append(jobRows, []string{j.Id, output.FormatEnumKebab(j.JobType), output.FormatEnum(j.Status), j.RunId, jobDuration(j), output.FormatAge(j.CreatedAt)})
			}
			d.Table("Recent Jobs", []string{"ID", "Type", "Status", "Run", "Duration", "Age"}, jobRows)

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral agent get "+a.Name)
		},
	}
	return cmd
}
