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

func newDeleteCmd(opts *client.Options) *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an agent",
		Long:  `Delete an agent. All associated service access tokens are revoked.`,
		Example: `  # Delete an agent (prompts to confirm)
  admiral agent delete prod-agent

  # Skip the confirmation prompt
  admiral agent delete prod-agent --force

  # Delete by UUID
  admiral agent delete <uuid>`,
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

			display := args[0]
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Delete agent %s and revoke all its tokens", display)); err != nil {
				return err
			}

			_, err = c.Agent().DeleteAgent(cmd.Context(), &agentv1.DeleteAgentRequest{
				AgentId: id,
			})
			if err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "agent", display, "deleted")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
