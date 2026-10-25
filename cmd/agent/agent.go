package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type AgentCmd struct {
	Cmd *cobra.Command
}

func NewAgentCmd(opts *client.Options) *AgentCmd {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Long: `Manage agents. An agent runs work where your infrastructure is: a
kubernetes agent applies manifests and Helm charts, an infra agent runs
Terraform or OpenTofu. Each runs as a service account made with it, and signs
in with a short-lived token from where it runs, so no long-lived secret exists.

An environment deploys through the agents assigned to it, and may be
assigned only an agent granted to the organization, a team, its application
or the environment itself.

Commands accept an agent name or ID as the positional argument.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(
		newListCmd(opts),
		newCreateCmd(opts),
		newGetCmd(opts),
		newDeleteCmd(opts),
		newEnrollmentKeyCmd(opts),
		newUpdateCmd(opts),
		newGrantCmd(opts),
		newRevokeCmd(opts),
	)
	return &AgentCmd{Cmd: cmd}
}
