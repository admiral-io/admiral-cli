package run

import (
	"fmt"
	"strconv"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

var runTable = output.Table[*runv1.Run]{
	{Header: "ID", Cell: func(r *runv1.Run) string { return r.GetId() }},
	{Header: "STATUS", Cell: describeStatus},
	{Header: "SOURCE", Cell: source},
	{Header: "CHANGES", Cell: func(r *runv1.Run) string { return changeSummary(r.GetPlan().GetComponents()) }},
	{Header: "AGE", Cell: func(r *runv1.Run) string { return output.FormatAge(r.GetCreatedAt()) }},
	{Header: "ENVIRONMENT", Wide: true, Cell: func(r *runv1.Run) string { return r.GetEnvironmentId() }},
	{Header: "CREATED BY", Wide: true, Cell: func(r *runv1.Run) string { return output.FormatActor(r.GetCreatedBy()) }},
}

func source(r *runv1.Run) string {
	if r.GetRevisionId() != "" {
		return r.GetRevisionId()
	}
	return output.FormatEnumKebab(r.GetDesired().GetMode())
}

// changeSummary counts resource changes the way a plan reads:
// +create ~update ±replace -destroy.
func changeSummary(cs []*agentv1.ComponentPlan) string {
	var add, change, replace, destroy int
	for _, c := range cs {
		for _, rc := range c.GetChanges() {
			switch rc.GetAction() {
			case agentv1.ResourceChange_ACTION_CREATE:
				add++
			case agentv1.ResourceChange_ACTION_UPDATE:
				change++
			case agentv1.ResourceChange_ACTION_REPLACE:
				replace++
			case agentv1.ResourceChange_ACTION_DESTROY, agentv1.ResourceChange_ACTION_ORPHAN:
				destroy++
			}
		}
	}
	if add+change+replace+destroy == 0 {
		return ""
	}
	return fmt.Sprintf("+%d ~%d ±%d -%d", add, change, replace, destroy)
}

// Print writes a run's detail view, or the run itself under -o json|yaml.
func Print(p *output.Printer, r *runv1.Run) error {
	d := output.NewDescribe()
	d.Field("ID", r.GetId())
	d.Field("Environment", r.GetEnvironmentId())
	if r.GetRevisionId() != "" {
		d.Field("Changeset", r.GetChangeSetId())
		d.Field("Revision", r.GetRevisionId())
	} else {
		d.Field("Desired", output.FormatEnumKebab(r.GetDesired().GetMode()))
	}
	d.Field("Status", describeStatus(r))
	d.Field("Trigger", output.FormatEnumKebab(r.GetTrigger()))
	d.Field("Created by", output.FormatActor(r.GetCreatedBy()))
	d.Field("Created", output.FormatDescribeTime(r.GetCreatedAt()))
	if r.GetFinishedAt() != nil {
		d.Field("Finished", output.FormatDescribeTime(r.GetFinishedAt()))
	}

	if plan := r.GetPlan(); plan != nil {
		d.Section("Plan", func(b *output.Block) {
			b.Field("Digest", plan.GetPlanDigest())
			if plan.GetMessage() != "" {
				b.Field("Message", plan.GetMessage())
			}
			if len(plan.GetComponents()) > 0 {
				rows := make([][]string, 0, len(plan.GetComponents()))
				for _, c := range plan.GetComponents() {
					rows = append(rows, []string{c.GetComponent(), changeSummary([]*agentv1.ComponentPlan{c})})
				}
				b.Table([]string{"Component", "Changes"}, rows)
			}
			jobTable(b, plan.GetJobs())
		})
	}
	if len(r.GetApprovals()) > 0 {
		rows := make([][]string, 0, len(r.GetApprovals()))
		for _, a := range r.GetApprovals() {
			rows = append(rows, []string{output.FormatActor(a.GetApprovedBy()), a.GetReason(), output.FormatAge(a.GetApprovedAt())})
		}
		d.Table("Approvals", []string{"By", "Reason", "Age"}, rows)
	}
	if apply := r.GetApply(); apply != nil {
		d.Section("Apply", func(b *output.Block) {
			b.Field("Applied by", output.FormatActor(apply.GetAppliedBy()))
			if apply.Generation != nil {
				b.Field("Generation", strconv.FormatInt(apply.GetGeneration(), 10))
			}
			if apply.GetMessage() != "" {
				b.Field("Message", apply.GetMessage())
			}
			if apply.GetAcceptedBy() != nil {
				b.Field("Accepted by", output.FormatActor(apply.GetAcceptedBy()))
				b.Field("Accepted reason", apply.GetAcceptedReason())
			}
			if len(apply.GetComponents()) > 0 {
				rows := make([][]string, 0, len(apply.GetComponents()))
				for _, c := range apply.GetComponents() {
					rows = append(rows, []string{c.GetComponent(), output.FormatEnumKebab(c.GetStatus()), c.GetMessage()})
				}
				b.Table([]string{"Component", "Status", "Message"}, rows)
			}
			jobTable(b, apply.GetJobs())
		})
	}
	switch r.GetStatus() {
	case runv1.Run_STATUS_PLANNED:
		d.Hint("To see the changes, run: admiral run diff " + r.GetId())
		d.Hint("To apply them, run: admiral run approve " + r.GetId() + " && admiral run apply " + r.GetId())
	case runv1.Run_STATUS_FAILED:
		d.Hint("To see the agent's output, run: admiral run logs " + r.GetId())
	}
	return p.PrintStatus(r, d)
}

func jobTable(b *output.Block, jobs []*runv1.RunJob) {
	if len(jobs) == 0 {
		return
	}
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		st := output.FormatEnumKebab(j.GetStatus())
		if j.GetWaitReason() != 0 {
			st += " (" + output.FormatEnumKebab(j.GetWaitReason()) + ")"
		}
		rows = append(rows, []string{j.GetJobId(), j.GetAgentId(), st, fmt.Sprint(len(j.GetComponents()))})
	}
	b.Table([]string{"Job", "Agent", "Status", "Components"}, rows)
}
