package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newTokenGetCmd(opts *client.Options) *cobra.Command {
	var agentName string

	cmd := &cobra.Command{
		Use:   "get <token>",
		Short: "Get agent token details",
		Long:  `Get metadata for an agent's service access token.`,
		Example: `  # Get a token by name (the agent is needed to look the name up)
  admiral agent token get rotate-2026 --agent prod-agent

  # Get a token by ID (no agent needed)
  admiral agent token get <uuid>`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			tokenID, err := resolve.AgentToken(cmd.Context(), c.Agent(), agentName, args[0])
			if err != nil {
				return err
			}

			resp, err := c.Agent().GetApiKey(cmd.Context(), &agentv1.GetApiKeyRequest{
				TokenId: tokenID,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.ApiKey, resp.ApiKey.Name, tokenTable.Render(p, resp.ApiKey))
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "", "agent name or ID (required to look a token up by name)")
	return cmd
}
