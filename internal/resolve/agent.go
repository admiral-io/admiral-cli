package resolve

import (
	"context"

	"go.admiral.io/cli/internal/cmderr"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

// Agent resolves an agent name or agt- id. The server's agent filter has no
// name, so names are matched against every agent; a tenant has few.
func Agent(ctx context.Context, c agentv1.AgentAPIClient, nameOrID string) (string, error) {
	if IsID("agt", nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.Usage("no agent specified")
	}
	var token string
	for {
		resp, err := c.ListAgents(ctx, &agentv1.ListAgentsRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return "", err
		}
		for _, a := range resp.GetAgents() {
			if a.GetName() == nameOrID {
				return a.GetId(), nil
			}
		}
		if token = resp.GetNextPageToken(); token == "" {
			return "", &NotFoundError{Kind: "agent", Name: nameOrID, ListCommand: "agent list"}
		}
	}
}

// Provider resolves a workload identity provider name or wip- id.
func Provider(ctx context.Context, c agentv1.WorkloadIdentityProviderAPIClient, nameOrID string) (string, error) {
	if IsID("wip", nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.Usage("no provider specified")
	}
	var token string
	for {
		resp, err := c.ListWorkloadIdentityProviders(ctx, &agentv1.ListWorkloadIdentityProvidersRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return "", err
		}
		for _, p := range resp.GetWorkloadIdentityProviders() {
			if p.GetName() == nameOrID {
				return p.GetId(), nil
			}
		}
		if token = resp.GetNextPageToken(); token == "" {
			return "", &NotFoundError{Kind: "provider", Name: nameOrID, ListCommand: "agent provider list"}
		}
	}
}
