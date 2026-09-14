package agent

import (
	"fmt"
	"strings"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

// parseAgentKind maps the user-facing kind string to the wire enum.
func parseAgentKind(s string) (agentv1.AgentKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "terraform", "tf", "tofu":
		return agentv1.AgentKind_AGENT_KIND_TERRAFORM, nil
	case "kubernetes", "k8s":
		return agentv1.AgentKind_AGENT_KIND_KUBERNETES, nil
	default:
		return agentv1.AgentKind_AGENT_KIND_UNSPECIFIED, fmt.Errorf("invalid kind %q (want terraform|kubernetes)", s)
	}
}

// jobTable renders agent jobs (`agent jobs`).
var jobTable = output.Table[*agentv1.Job]{
	{Header: "ID", Cell: func(j *agentv1.Job) string { return j.Id }},
	{Header: "TYPE", Cell: func(j *agentv1.Job) string { return output.FormatEnumKebab(j.JobType) }},
	{Header: "STATUS", Cell: func(j *agentv1.Job) string { return output.FormatEnum(j.Status) }},
	{Header: "RUN", Cell: func(j *agentv1.Job) string { return j.RunId }},
	{Header: "DURATION", Cell: jobDuration},
	{Header: "AGE", Cell: func(j *agentv1.Job) string { return output.FormatAge(j.CreatedAt) }},
	{Header: "REVISION", Wide: true, Cell: func(j *agentv1.Job) string { return j.RevisionId }},
	{Header: "STARTED", Wide: true, Cell: func(j *agentv1.Job) string { return output.FormatTimestamp(j.StartedAt) }},
	{Header: "COMPLETED", Wide: true, Cell: func(j *agentv1.Job) string { return output.FormatTimestamp(j.CompletedAt) }},
}

var jobStatusFromString = map[string]agentv1.JobStatus{
	"pending":   agentv1.JobStatus_JOB_STATUS_PENDING,
	"assigned":  agentv1.JobStatus_JOB_STATUS_ASSIGNED,
	"running":   agentv1.JobStatus_JOB_STATUS_RUNNING,
	"succeeded": agentv1.JobStatus_JOB_STATUS_SUCCEEDED,
	"failed":    agentv1.JobStatus_JOB_STATUS_FAILED,
	"canceled":  agentv1.JobStatus_JOB_STATUS_CANCELED,
}

var jobTypeFromString = map[string]agentv1.JobType{
	"plan":          agentv1.JobType_JOB_TYPE_PLAN,
	"apply":         agentv1.JobType_JOB_TYPE_APPLY,
	"destroy-plan":  agentv1.JobType_JOB_TYPE_DESTROY_PLAN,
	"destroy_plan":  agentv1.JobType_JOB_TYPE_DESTROY_PLAN,
	"destroy-apply": agentv1.JobType_JOB_TYPE_DESTROY_APPLY,
	"destroy_apply": agentv1.JobType_JOB_TYPE_DESTROY_APPLY,
}

// agentTable is the single column definition shared by list, get and the
// create/update echo.
var agentTable = output.Table[*agentv1.Agent]{
	{Header: "NAME", Cell: func(a *agentv1.Agent) string { return a.Name }},
	{Header: "KIND", Cell: func(a *agentv1.Agent) string { return output.FormatEnumKebab(a.Kind) }},
	{Header: "HEALTH", Cell: func(a *agentv1.Agent) string { return output.FormatEnum(a.HealthStatus) }},
	{Header: "DESCRIPTION", Truncate: 40, Cell: func(a *agentv1.Agent) string { return a.Description }},
	{Header: "LABELS", Cell: func(a *agentv1.Agent) string { return output.FormatLabels(a.Labels) }},
	{Header: "AGE", Cell: func(a *agentv1.Agent) string { return output.FormatAge(a.CreatedAt) }},
	{Header: "ID", Wide: true, Cell: func(a *agentv1.Agent) string { return a.Id }},
	{Header: "CREATED BY", Wide: true, Cell: func(a *agentv1.Agent) string { return output.FormatActor(a.CreatedBy) }},
}
