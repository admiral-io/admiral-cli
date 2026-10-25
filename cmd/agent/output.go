package agent

import (
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

var agentTable = output.Table[*agentv1.Agent]{
	{Header: "NAME", Cell: func(a *agentv1.Agent) string { return a.GetName() }},
	{Header: "KIND", Cell: func(a *agentv1.Agent) string { return output.FormatEnumKebab(a.GetKind()) }},
	{Header: "HEALTH", Cell: func(a *agentv1.Agent) string { return output.FormatEnum(a.GetHealth()) }},
	{Header: "LAST SEEN", Cell: lastSeen},
	{Header: "VERSION", Cell: func(a *agentv1.Agent) string { return a.GetReport().GetAgentVersion() }},
	{Header: "AGE", Cell: func(a *agentv1.Agent) string { return output.FormatAge(a.GetCreatedAt()) }},
	{Header: "ID", Wide: true, Cell: func(a *agentv1.Agent) string { return a.GetId() }},
	{Header: "SERVICE ACCOUNT", Wide: true, Cell: func(a *agentv1.Agent) string { return a.GetServiceAccountId() }},
}

// lastSeen is how long ago the agent last reported, so an offline agent
// says how long it has been gone.
func lastSeen(a *agentv1.Agent) string {
	if a.GetReport().GetReportedAt() == nil {
		return "never"
	}
	return output.FormatAge(a.GetReport().GetReportedAt()) + " ago"
}

func grantTarget(g *agentv1.AgentGrant) string {
	switch {
	case g.GetOrganization() != nil:
		return "org"
	case g.GetGroupId() != "":
		return "group:" + g.GetGroupId()
	case g.GetApplicationId() != "":
		return "app:" + g.GetApplicationId()
	default:
		return "env:" + g.GetEnvironmentId()
	}
}
