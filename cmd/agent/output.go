package agent

import (
	"strings"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

var agentTable = output.Table[*agentv1.Agent]{
	{Header: "NAME", Cell: func(a *agentv1.Agent) string { return a.GetName() }},
	{Header: "KIND", Cell: func(a *agentv1.Agent) string { return output.FormatEnumKebab(a.GetKind()) }},
	{Header: "HEALTH", Cell: func(a *agentv1.Agent) string { return output.FormatEnum(a.GetHealth()) }},
	{Header: "VERSION", Cell: func(a *agentv1.Agent) string { return a.GetReport().GetAgentVersion() }},
	{Header: "AGE", Cell: func(a *agentv1.Agent) string { return output.FormatAge(a.GetCreatedAt()) }},
	{Header: "ID", Wide: true, Cell: func(a *agentv1.Agent) string { return a.GetId() }},
	{Header: "SUBJECT", Wide: true, Cell: func(a *agentv1.Agent) string { return a.GetIdentity().GetWorkload().GetSubject() }},
}

var providerTable = output.Table[*agentv1.WorkloadIdentityProvider]{
	{Header: "NAME", Cell: func(p *agentv1.WorkloadIdentityProvider) string { return p.GetName() }},
	{Header: "STATUS", Cell: func(p *agentv1.WorkloadIdentityProvider) string { return output.FormatEnum(p.GetStatus()) }},
	{Header: "KEYS", Cell: func(p *agentv1.WorkloadIdentityProvider) string { return keysSource(p) }},
	{Header: "PENDING KEYS", Cell: func(p *agentv1.WorkloadIdentityProvider) string { return strings.Join(p.GetPendingKeyIds(), ",") }},
	{Header: "AGE", Cell: func(p *agentv1.WorkloadIdentityProvider) string { return output.FormatAge(p.GetCreatedAt()) }},
	{Header: "ID", Wide: true, Cell: func(p *agentv1.WorkloadIdentityProvider) string { return p.GetId() }},
	{Header: "ISSUER", Wide: true, Cell: func(p *agentv1.WorkloadIdentityProvider) string { return p.GetIssuer() }},
}

func keysSource(p *agentv1.WorkloadIdentityProvider) string {
	switch {
	case p.GetKeys().GetJwksUri() != "":
		return "url"
	case p.GetKeys().GetJwks() != "":
		return "pasted"
	default:
		return "enrollment"
	}
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
