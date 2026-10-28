package agent

import (
	"context"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	serviceaccountv1 "go.admiral.io/sdk/proto/admiral/api/serviceaccount/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <agent>",
		Short: "Show everything about an agent",
		Long: `Render the operator view of an agent: what it last reported, whether it
has enrolled and as whom, and who may use it. A section the active
credential cannot read is shown as unavailable.

describe is a human view. Use 'agent get -o json' for the raw record.`,
		Example: `  admiral agent describe dev`,
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			id, err := resolve.Agent(cmd.Context(), c.Agent(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Agent().GetAgent(cmd.Context(), &agentv1.GetAgentRequest{AgentId: id})
			if err != nil {
				return err
			}
			a := resp.GetAgent()
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if !p.Format.IsTable() {
				return p.PrintDescribe(nil, "admiral agent get "+a.GetName())
			}
			d, err := describeAgent(cmd.Context(), c, a)
			if err != nil {
				return err
			}
			return p.PrintDescribe(d, "admiral agent get "+a.GetName())
		},
	}
}

func describeAgent(ctx context.Context, c sdkclient.AdmiralClient, a *agentv1.Agent) (*output.Describe, error) {
	d := output.NewDescribe()
	d.Field("Name", a.GetName())
	d.Field("ID", a.GetId())
	d.Field("Kind", output.FormatEnumKebab(a.GetKind()))
	d.Field("Health", output.FormatEnum(a.GetHealth()))
	d.Field("Last Seen", lastSeen(a))
	d.Field("Version", a.GetReport().GetAgentVersion())
	if k := a.GetReport().GetKubernetes(); k != nil {
		d.Field("Kubernetes", k.GetKubeVersion())
	}
	d.Field("Created", output.FormatDescribeTime(a.GetCreatedAt()))
	d.Field("Created By", output.FormatActorWithID(a.GetCreatedBy()))

	grantsDenied, err := addGrants(ctx, d, c, a)
	if err != nil {
		return nil, err
	}
	denied, err := addServiceAccount(ctx, d, c, a)
	if err != nil {
		return nil, err
	}
	if denied || grantsDenied {
		d.Hint(cmderr.ScopeHint)
	}
	return d, nil
}

// addServiceAccount follows the agent to its service account, which is
// where enrollment records the issuer and subject it signs in with. It
// reports whether the read was denied.
func addServiceAccount(ctx context.Context, d *output.Describe, c sdkclient.AdmiralClient, a *agentv1.Agent) (bool, error) {
	resp, err := c.ServiceAccount().GetServiceAccount(ctx,
		&serviceaccountv1.GetServiceAccountRequest{ServiceAccountId: a.GetServiceAccountId()})
	denied := status.Code(err) == codes.PermissionDenied
	if err != nil && !denied {
		return false, err
	}
	sa := resp.GetServiceAccount()
	d.Section("Service Account", func(b *output.Block) {
		b.Field("ID", a.GetServiceAccountId())
		switch {
		case denied:
			// Reading an agent does not imply reading its service account.
			b.Field("Enrolled", "needs serviceaccount:read")
		case sa.GetEnrolledAt() == nil:
			b.Field("Enrolled", "not enrolled")
		default:
			b.Field("Enrolled", output.FormatDescribeTime(sa.GetEnrolledAt()))
			b.Field("Subject", sa.GetSubject())
			b.Field("Issuer", sa.GetIssuerId())
		}
	})
	return denied, nil
}

// addGrants lists who may use the agent, applications and environments by
// name. It reports whether the read was denied.
func addGrants(ctx context.Context, d *output.Describe, c sdkclient.AdmiralClient, a *agentv1.Agent) (bool, error) {
	resp, err := c.Agent().ListAgentGrants(ctx, &agentv1.ListAgentGrantsRequest{AgentId: a.GetId()})
	switch {
	case status.Code(err) == codes.PermissionDenied:
		d.Unavailable("Granted To", cmderr.Format(err))
		return true, nil
	case err != nil:
		return false, err
	}
	targets := make([]string, 0, len(resp.GetGrants()))
	for _, g := range resp.GetGrants() {
		targets = append(targets, grantName(ctx, c, g))
	}
	d.Fields("Granted To", targets)
	return false, nil
}

// grantName is a grant's target as 'agent grant' takes it, with names in
// place of ids where they can be read.
func grantName(ctx context.Context, c sdkclient.AdmiralClient, g *agentv1.AgentGrant) string {
	switch {
	case g.GetApplicationId() != "":
		if resp, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: g.GetApplicationId()}); err == nil {
			return "app:" + resp.GetApplication().GetName()
		}
	case g.GetEnvironmentId() != "":
		env, err := c.Environment().GetEnvironment(ctx, &environmentv1.GetEnvironmentRequest{EnvironmentId: g.GetEnvironmentId()})
		if err != nil {
			break
		}
		app, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: env.GetEnvironment().GetApplicationId()})
		if err == nil {
			return "env:" + app.GetApplication().GetName() + "/" + env.GetEnvironment().GetName()
		}
	}
	return grantTarget(g)
}
