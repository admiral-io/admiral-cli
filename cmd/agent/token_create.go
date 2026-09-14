package agent

import (
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newTokenCreateCmd(opts *client.Options) *cobra.Command {
	var (
		agentName string
		expiresIn time.Duration
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent service access token",
		Long: `Create an additional service access token (SAT) for an agent.

The token secret is printed once. Store it securely -- it cannot be retrieved again.`,
		Example: `  # Create a token for an agent
  admiral agent token create rotate-2026 --agent prod-agent

  # Scope by agent UUID and set a 30-day expiry
  admiral agent token create rotate-2026 --agent prod-agent --expires-in 720h`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Agent(cmd.Context(), c.Agent(), agentName)
			if err != nil {
				return err
			}

			req := &agentv1.CreateApiKeyRequest{
				AgentId: id,
				Name:    args[0],
			}
			if expiresIn > 0 {
				req.ExpiresAt = timestamppb.New(time.Now().Add(expiresIn))
			}

			resp, err := c.Agent().CreateApiKey(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			if err := p.PrintResource(resp, tokenTable.Render(p, resp.ApiKey)); err != nil {
				return err
			}

			if opts.OutputFormat.IsTable() {
				output.Writef(cmd.ErrOrStderr(), "\nSAT (shown once, store securely):\n%s\n", resp.PlainTextKey)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "", "agent name or ID (required)")
	_ = cmd.MarkFlagRequired("agent")
	cmd.Flags().DurationVar(&expiresIn, "expires-in", 0, "expiry duration (e.g. 720h); unset = no expiry")
	return cmd
}
