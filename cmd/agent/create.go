package agent

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		kind        string
		description string
		labelStrs   []string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent",
		Long: `Create an agent and issue its initial service access token (SAT).

The --kind selects the execution contract: 'terraform' (infrastructure runner) or
'kubernetes' (cluster agent). The SAT is printed once. Deploy it to the
agent binary so the agent can authenticate to Admiral. Additional tokens can be
issued later via 'admiral agent token create' for zero-downtime rotation.`,
		Example: `  # Create a terraform (infrastructure) agent (default kind)
  admiral agent create prod-runner

  # Create a kubernetes agent
  admiral agent create prod-cluster --kind kubernetes

  # With a description and labels
  admiral agent create prod-runner --description "AWS production runner" --label team=platform`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentKind, err := parseAgentKind(kind)
			if err != nil {
				return err
			}

			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.Agent().CreateAgent(cmd.Context(), &agentv1.CreateAgentRequest{
				Kind:        agentKind,
				Name:        args[0],
				Description: description,
				Labels:      labels,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			if err := p.PrintResource(resp, agentTable.Render(p, resp.Agent)); err != nil {
				return err
			}

			if opts.OutputFormat.IsTable() {
				output.Writef(cmd.ErrOrStderr(), "\nSAT (shown once, store securely):\n%s\n", resp.PlainTextKey)
			}
			return nil
		},
	}

	flags.Enum(cmd, &kind, "kind", "terraform", "agent kind", "terraform", "kubernetes")
	cmd.Flags().StringVar(&description, "description", "", "agent description")
	flags.Label(cmd, &labelStrs, "label to attach (key=value, repeatable)")
	return cmd
}
