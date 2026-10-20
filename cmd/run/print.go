package run

import (
	"fmt"
	"strconv"

	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
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
// title is its changeset's, "" when unknown.
func Print(p *output.Printer, r *runv1.Run, title string) error {
	return printRun(p, r, title, false)
}

// PrintBeforeDiff is Print for a caller that writes the plan's diff right
// after, so the view does not point at 'run diff'.
func PrintBeforeDiff(p *output.Printer, r *runv1.Run, title string) error {
	return printRun(p, r, title, true)
}

// ChangeSetLabel is a changeset's id with its title, when known.
func ChangeSetLabel(id, title string) string {
	if title == "" {
		return id
	}
	return fmt.Sprintf("%s %q", id, title)
}

func printRun(p *output.Printer, r *runv1.Run, title string, diffShown bool) error {
	d := output.NewDescribe()
	d.Field("ID", r.GetId())
	d.Field("Environment", r.GetEnvironmentId())
	if r.GetRevisionId() != "" {
		d.Field("Changeset", ChangeSetLabel(r.GetChangeSetId(), title))
		d.Field("Revision", r.GetRevisionId())
	} else {
		d.Field("Desired", output.FormatEnumKebab(r.GetDesired().GetMode()))
	}
	d.Field("Status", describeStatus(r))
	d.Field("Trigger", output.FormatEnumKebab(r.GetTrigger()))
	d.Field("Created by", output.FormatActorWithID(r.GetCreatedBy()))
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
			b.Field("Applied by", output.FormatActorWithID(apply.GetAppliedBy()))
			if apply.Generation != nil {
				b.Field("Generation", strconv.FormatInt(apply.GetGeneration(), 10))
			}
			if apply.GetMessage() != "" {
				b.Field("Message", apply.GetMessage())
			}
			if apply.GetAcceptedBy() != nil {
				b.Field("Accepted by", output.FormatActorWithID(apply.GetAcceptedBy()))
				b.Field("Accepted reason", apply.GetAcceptedReason())
			}
			if len(apply.GetComponents()) > 0 {
				planned := map[string]*agentv1.ComponentPlan{}
				for _, c := range r.GetPlan().GetComponents() {
					planned[c.GetComponent()] = c
				}
				rows := make([][]string, 0, len(apply.GetComponents()))
				for _, c := range apply.GetComponents() {
					changes := ""
					if pc := planned[c.GetComponent()]; pc != nil && c.GetStatus() == runv1.ComponentApply_STATUS_SUCCEEDED {
						changes = changeSummary([]*agentv1.ComponentPlan{pc})
					}
					rows = append(rows, []string{c.GetComponent(), output.FormatEnumKebab(c.GetStatus()), changes, c.GetMessage()})
				}
				b.Table([]string{"Component", "Status", "Changes", "Message"}, rows)
			}
			jobTable(b, apply.GetJobs())
		})
	}
	switch r.GetStatus() {
	case runv1.Run_STATUS_PLANNED:
		if !diffShown {
			d.Hint("To see the changes, run: admiral run diff " + r.GetId())
		}
		for _, h := range nextSteps(r) {
			d.Hint(h)
		}
	case runv1.Run_STATUS_FAILED:
		d.Hint("To see the agent's output, run: admiral run logs " + r.GetId())
	}
	return p.PrintStatus(r, d)
}

// nextSteps says how to act on a planned run: through its changeset when it
// has one, approving first only when the environment asks for approvals.
func nextSteps(r *runv1.Run) []string {
	verb, target := "admiral run", r.GetId()
	if r.GetChangeSetId() != "" {
		verb, target = "admiral changeset", r.GetChangeSetId()
	}
	var steps []string
	if n := requiredApprovals(r.GetRequiredApproval()); n > 0 {
		steps = append(steps, fmt.Sprintf(
			"It needs %d approval(s) from someone other than the changeset's author: %s approve %s", n, verb, target))
	}
	return append(steps, fmt.Sprintf("To apply it, run: %s apply %s", verb, target))
}

// requiredApprovals is the most approvals the rule asks of anyone: its own
// count or a group's.
func requiredApprovals(rule *environmentv1.ApprovalRule) int32 {
	n := rule.GetRequiredApprovals()
	for _, g := range rule.GetGroups() {
		n = max(n, g.GetApprovals())
	}
	return n
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
