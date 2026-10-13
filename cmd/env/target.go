package env

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// targetFlags are where an environment deploys and what an apply there
// needs, as flags.
type targetFlags struct {
	agents    []string
	namespace string
	approvals int32
}

func (t *targetFlags) register(cmd *cobra.Command, agentUsage, namespaceUsage string) {
	cmd.Flags().StringArrayVar(&t.agents, "agent", nil, agentUsage)
	cmd.Flags().StringVar(&t.namespace, "namespace", "", namespaceUsage)
	cmd.Flags().Int32Var(&t.approvals, "approvals", 0, "approvals an apply needs before it runs (0 for none)")
}

// check refuses what is wrong in the flags alone, before any sign-in. On
// create the flags are the whole agent list, so a namespace without one
// agent is known to be wrong here.
func (t *targetFlags) check(cmd *cobra.Command, creating bool) error {
	if cmd.Flags().Changed("namespace") {
		if t.namespace != "" && !namespacePattern.MatchString(t.namespace) {
			return cmderr.Usage("invalid namespace %q: at most 63 lowercase letters, digits and hyphens", t.namespace)
		}
		if (creating || cmd.Flags().Changed("agent")) && len(t.agents) != 1 {
			return cmderr.Usage("--namespace needs exactly one --agent, got %d", len(t.agents))
		}
	}
	if t.approvals < 0 {
		return cmderr.Usage("--approvals cannot be negative")
	}
	return nil
}

// apply writes the flags that were given onto e, resolving agent names, and
// returns the update mask paths they set. An agent already assigned keeps
// its assignment id and settings, so components placed on it stay valid.
func (t *targetFlags) apply(ctx context.Context, cmd *cobra.Command, agents agentv1.AgentAPIClient, e *environmentv1.Environment) ([]string, error) {
	var paths []string
	if cmd.Flags().Changed("agent") {
		current := make(map[string]*environmentv1.AgentAssignment, len(e.Agents))
		for _, a := range e.Agents {
			current[a.AgentId] = a
		}
		next := make([]*environmentv1.AgentAssignment, 0, len(t.agents))
		seen := map[string]bool{}
		for _, ref := range t.agents {
			id, err := resolve.Agent(ctx, agents, ref)
			if err != nil {
				return nil, err
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			if a, ok := current[id]; ok {
				next = append(next, a)
			} else {
				next = append(next, &environmentv1.AgentAssignment{AgentId: id})
			}
		}
		e.Agents = next
		paths = append(paths, "agents")
	}
	if cmd.Flags().Changed("namespace") {
		if len(e.Agents) != 1 {
			return nil, cmderr.Usage("--namespace sets the namespace of an environment's one agent; this one has %d", len(e.Agents))
		}
		a := e.Agents[0]
		k := a.GetKubernetes()
		if k == nil {
			k = &environmentv1.KubernetesSettings{}
			a.Settings = &environmentv1.AgentAssignment_Kubernetes{Kubernetes: k}
		}
		k.DefaultNamespace = t.namespace
		if !slices.Contains(paths, "agents") {
			paths = append(paths, "agents")
		}
	}
	if cmd.Flags().Changed("approvals") {
		if e.Approval == nil {
			e.Approval = &environmentv1.ApprovalRule{}
		}
		e.Approval.RequiredApprovals = t.approvals
		paths = append(paths, "approval")
	}
	return paths, nil
}

// agentIDs is the assignments as one cell.
func agentIDs(e *environmentv1.Environment) string {
	ids := make([]string, 0, len(e.Agents))
	for _, a := range e.Agents {
		ids = append(ids, a.AgentId)
	}
	return strings.Join(ids, ",")
}

// namespaces is the Kubernetes default namespaces as one cell.
func namespaces(e *environmentv1.Environment) string {
	var ns []string
	for _, a := range e.Agents {
		if k := a.GetKubernetes(); k != nil && k.DefaultNamespace != "" {
			ns = append(ns, k.DefaultNamespace)
		}
	}
	return strings.Join(ns, ",")
}

func approvals(e *environmentv1.Environment) string {
	return strconv.Itoa(int(e.GetApproval().GetRequiredApprovals()))
}
