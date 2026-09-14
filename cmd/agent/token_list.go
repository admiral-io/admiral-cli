package agent

import (
	"strings"

	commonv1 "buf.build/gen/go/admiral/common/protocolbuffers/go/admiral/common/v1"
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newTokenListCmd(opts *client.Options) *cobra.Command {
	var (
		agentName string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent tokens",
		Long:  `List all service access tokens bound to an agent.`,
		Example: `  # List tokens for an agent
  admiral agent token list --agent prod-agent`,
		Args: flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Agent(cmd.Context(), c.Agent(), agentName)
			if err != nil {
				return err
			}

			resp, err := c.Agent().ListApiKeys(cmd.Context(), &agentv1.ListApiKeysRequest{
				AgentId:   id,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "tokens",
				Items:         output.Messages(resp.ApiKeys),
				Name:          func(i int) string { return resp.ApiKeys[i].Name },
				NextPageToken: resp.NextPageToken,
			}, tokenTable.Render(p, resp.ApiKeys...))
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "", "agent name or ID (required)")
	_ = cmd.MarkFlagRequired("agent")
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")
	return cmd
}

func formatExpiry(t *commonv1.ApiKey) string {
	if t.ExpiresAt == nil {
		return "never"
	}
	return output.FormatTimestamp(t.ExpiresAt)
}

// tokenTable is the single column definition shared by token list, get and
// the create echo.
var tokenTable = output.Table[*commonv1.ApiKey]{
	{Header: "NAME", Cell: func(t *commonv1.ApiKey) string { return t.Name }},
	{Header: "PREFIX", Cell: func(t *commonv1.ApiKey) string { return t.KeyPrefix }},
	{Header: "STATUS", Cell: func(t *commonv1.ApiKey) string { return output.FormatEnum(t.Status) }},
	{Header: "EXPIRES", Cell: formatExpiry},
	{Header: "AGE", Cell: func(t *commonv1.ApiKey) string { return output.FormatAge(t.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(t *commonv1.ApiKey) string { return t.Id }},
	{Header: "SCOPES", Wide: true, Cell: func(t *commonv1.ApiKey) string { return strings.Join(t.Scopes, ",") }},
	{Header: "REVOKED", Wide: true, Cell: func(t *commonv1.ApiKey) string { return output.FormatTimestamp(t.RevokedAt) }},
}
