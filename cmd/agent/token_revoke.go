package agent

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newTokenRevokeCmd(opts *client.Options) *cobra.Command {
	var (
		agentName string
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "revoke <token>",
		Short: "Revoke an agent token",
		Long: `Revoke a service access token. The agent using it gets 401 on its next
request. Revocation cannot be undone; create a new token instead.`,
		Example: `  # Revoke a token by name (prompts to confirm)
  admiral agent token revoke rotate-2026 --agent prod-agent

  # Revoke a token by ID
  admiral agent token revoke <uuid>

  # Skip the prompt
  admiral agent token revoke <uuid> --force`,
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

			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Revoke token %s (the agent using it will get 401 on its next request)", args[0])); err != nil {
				return err
			}

			if _, err := c.Agent().RevokeApiKey(cmd.Context(), &agentv1.RevokeApiKeyRequest{
				TokenId: tokenID,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "token", args[0], "revoked")
			return nil
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "", "agent name or ID (required to look a token up by name)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
