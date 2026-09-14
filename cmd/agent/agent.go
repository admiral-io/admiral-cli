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
	root := &AgentCmd{}

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Long: `Manage execution agents. An agent is either a TERRAFORM agent (an
infrastructure runner that executes plan/apply/destroy jobs) or a KUBERNETES agent
(a cluster that applies workload revisions). Set the kind with --kind on create.`,
		Aliases:       []string{"agents"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newDescribeCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
		newStatusCmd(opts),
		newJobsCmd(opts),
		newTokenCmd(opts),
	)

	root.Cmd = cmd
	return root
}
