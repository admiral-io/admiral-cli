package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newGetCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get an agent",
		Long: `Print the one-line summary of an agent. Use 'describe' for the full view and '-o json' for the raw record.

The agent is given by name or ID.`,
		Example: `  # Get an agent by name
  admiral agent get prod-agent

  # Get an agent by UUID
  admiral agent get <uuid>`,
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

			resp, err := c.Agent().GetAgent(cmd.Context(), &agentv1.GetAgentRequest{
				AgentId: id,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Agent, resp.Agent.Name, agentTable.Render(p, resp.Agent))
		},
	}

	return cmd
}
