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
		Short: "Manage agents and the providers they sign in through",
		Long: `Manage agents. An agent runs work where your infrastructure is: a
Kubernetes cluster, or a Terraform runner. It signs in with a short-lived
token its workload identity provider issues, so no long-lived secret exists.

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
		newGrantCmd(opts),
		newGrantsCmd(opts),
		newRevokeCmd(opts),
		newProviderCmd(opts),
	)
	return &AgentCmd{Cmd: cmd}
}
