package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

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
	serviceaccountv1 "go.admiral.io/sdk/proto/admiral/api/serviceaccount/v1"
)

func newListCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents",
		Args:  flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			agents, next, err := flags.Pages(paging, func(token string) ([]*agentv1.Agent, string, error) {
				resp, err := c.Agent().ListAgents(cmd.Context(), &agentv1.ListAgentsRequest{PageSize: paging.PageSize, PageToken: token})
				if err != nil {
					return nil, "", err
				}
				return resp.GetAgents(), resp.GetNextPageToken(), nil
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind: "agents", Items: output.Messages(agents), NextPageToken: next,
				Name: func(i int) string { return agents[i].GetName() },
			}, agentTable.Render(p, agents...))
		},
	}
	flags.Paging(cmd, &paging)
	return cmd
}

func newGetCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <agent>",
		Short: "Get an agent, with what it last reported and whether it has enrolled",
		Args:  flags.ExactArgs(1),
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
			if err := p.PrintOne(a, a.GetName(), agentTable.Render(p, a)); err != nil {
				return err
			}
			if !opts.OutputFormat.IsTable() {
				return nil
			}
			return writeServiceAccount(cmd.Context(), p.Out(), c, a)
		},
	}
}

// writeServiceAccount follows the agent to its service account, which is
// where enrollment records the issuer and subject it signs in with.
func writeServiceAccount(ctx context.Context, w io.Writer, c sdkclient.AdmiralClient, a *agentv1.Agent) error {
	resp, err := c.ServiceAccount().GetServiceAccount(ctx,
		&serviceaccountv1.GetServiceAccountRequest{ServiceAccountId: a.GetServiceAccountId()})
	if err != nil && status.Code(err) != codes.PermissionDenied {
		return err
	}
	sa := resp.GetServiceAccount()
	d := output.NewDescribe()
	d.Field("Service Account", a.GetServiceAccountId())
	switch {
	case err != nil:
		// Reading an agent does not imply reading its service account.
		d.Field("Enrolled", "needs serviceaccount:read")
	case sa.GetEnrolledAt() == nil:
		d.Field("Enrolled", "not enrolled")
	default:
		d.Field("Enrolled", output.FormatDescribeTime(sa.GetEnrolledAt()))
		d.Field("Subject", sa.GetSubject())
		d.Field("Issuer", sa.GetIssuerId())
	}
	fmt.Fprintln(w)
	return d.Render(w)
}

var agentKinds = map[string]agentv1.Agent_Kind{
	"kubernetes": agentv1.Agent_KIND_KUBERNETES,
	"infra":      agentv1.Agent_KIND_INFRA,
}

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		kind     string
		grants   []string
		profiles []string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent and its one-time enrollment key",
		Long: `Create an agent. Admiral makes the service account it runs as, and
returns a single-use enrollment key. Install the agent with the key where it
runs; on its first start it enrolls, and Admiral records the issuer and
subject it signs in with from then on.

A kubernetes agent is installed with Helm into its own namespace,
admiral-system, never one it deploys into. The command prints the install.

The tenant's first agent is granted to the whole organization unless
--grant says otherwise; later ones serve nobody until granted.`,
		Example: `  admiral agent create dev --kind kubernetes

  # Granted to one application only
  admiral agent create prod --kind kubernetes --grant app:shop`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			req := &agentv1.CreateAgentRequest{Agent: &agentv1.Agent{Name: args[0], Kind: agentKinds[kind]}}
			for _, g := range grants {
				grant, err := parseGrant(cmd, c, g, profiles)
				if err != nil {
					return err
				}
				req.Grants = append(req.Grants, grant)
			}
			return createAgent(cmd, c, opts.OutputFormat, req)
		},
	}
	flags.Enum(cmd, &kind, "kind", "", "what the agent deploys", "kubernetes", "infra")
	cmd.Flags().StringArrayVar(&grants, "grant", nil, "grant use to org, app:<name>, env:<app/env> or group:<id> (repeatable)")
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "profiles the grants cover (repeatable; none covers all)")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func createAgent(cmd *cobra.Command, c sdkclient.AdmiralClient, format output.Format, req *agentv1.CreateAgentRequest) error {
	resp, err := c.Agent().CreateAgent(cmd.Context(), req)
	if err != nil {
		return err
	}
	p := output.NewPrinter(cmd, format)
	if err := p.PrintResource(resp, agentTable.Render(p, resp.GetAgent())); err != nil {
		return err
	}
	if !format.IsTable() {
		return nil
	}
	writeEnrollment(p.Out(), resp.GetAgent(), resp.GetEnrollmentKey())
	return nil
}

// agentChart is the chart a kubernetes agent installs from. The OCI
// reference needs no helm repo add.
const agentChart = "oci://ghcr.io/admiral-io/admiral-helm/admiral-k8s-agent"

// writeEnrollment prints the agent's id, its key with the expiry the server
// set, and for a kubernetes agent the Helm install that enrolls it.
func writeEnrollment(w io.Writer, a *agentv1.Agent, key *agentv1.EnrollmentKey) {
	fmt.Fprintf(w, "\nAgent %s created.\n", a.GetId())
	fmt.Fprintf(w, "\nEnrollment key (single use, expires %s):\n%s\n",
		output.FormatDescribeTime(key.GetExpiresAt()), key.GetSecret())
	if a.GetKind() != agentv1.Agent_KIND_KUBERNETES {
		fmt.Fprintln(w, "\nInstalling an infra agent is not covered here yet. Give it this key on its first start.")
		return
	}
	// The key goes in a Secret, not a --set, so it never lands in the
	// release's stored values.
	// The release name holds the chart's name, so the chart uses it as is
	// rather than appending its own: the Kubernetes service account, and so
	// the subject the agent enrolls with, is admiral-k8s-agent-<name>.
	release := "admiral-k8s-agent-" + a.GetName()
	secret := release + "-enrollment"
	fmt.Fprintf(w, `
Install it where it runs, before the key expires:

  kubectl create namespace admiral-system
  kubectl -n admiral-system create secret generic %s \
    --from-literal=key=%s
  helm install %s %s \
    --namespace admiral-system \
    --set admiral.serviceAccountId=%s \
    --set enrollment.existingSecret=%s
`, secret, key.GetSecret(), release, agentChart, a.GetServiceAccountId(), secret)
}

func newDeleteCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <agent>",
		Short: "Delete an agent",
		Long: `Delete an agent. Its running jobs end at once. Refused while an environment
has it assigned: remove the assignment first.`,
		Args: flags.ExactArgs(1),
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
			if _, err := c.Agent().DeleteAgent(cmd.Context(), &agentv1.DeleteAgentRequest{AgentId: id}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "agent %s deleted\n", args[0])
			return nil
		},
	}
}

func newEnrollmentKeyCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enrollment-key <agent>",
		Short: "Issue a new single-use enrollment key",
		Long: `Issue a single-use key the agent enrolls with, for one that has not
enrolled or must enroll again (reinstalled under another Kubernetes service
account, say). It replaces any key issued before.`,
		Args: flags.ExactArgs(1),
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
			resp, err := c.Agent().CreateEnrollmentKey(cmd.Context(), &agentv1.CreateEnrollmentKeyRequest{AgentId: id})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.GetEnrollmentKey().GetSecret())
			return nil
		},
	}
}

func newGrantCmd(opts *client.Options) *cobra.Command {
	var profiles []string
	cmd := &cobra.Command{
		Use:   "grant <agent> <org|app:NAME|env:APP/ENV|group:ID>",
		Short: "Let the organization, an application, an environment or a team use an agent",
		Long: `Grant use of an agent. Granting the same target again replaces its
profiles. Without --profile the grant covers every profile the agent has.`,
		Example: `  admiral agent grant kind org
  admiral agent grant prod app:shop --profile read --profile edit`,
		Args: flags.ExactArgs(2),
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
			g, err := parseGrant(cmd, c, args[1], profiles)
			if err != nil {
				return err
			}
			if _, err := c.Agent().GrantAgentUse(cmd.Context(), &agentv1.GrantAgentUseRequest{AgentId: id, Grant: g}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "agent %s granted to %s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "profiles the grant covers (repeatable; none covers all)")
	return cmd
}

func newGrantsCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "grants <agent>",
		Short: "List who may use an agent",
		Args:  flags.ExactArgs(1),
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
			resp, err := c.Agent().ListAgentGrants(cmd.Context(), &agentv1.ListAgentGrantsRequest{AgentId: id})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			grants := resp.GetGrants()
			return p.PrintList(output.List{
				Kind: "grants", Items: output.Messages(grants),
				Name: func(i int) string { return grantTarget(grants[i]) },
			}, grantTable.Render(p, grants...))
		},
	}
}

var grantTable = output.Table[*agentv1.AgentGrant]{
	{Header: "TARGET", Cell: grantTarget},
	{Header: "PROFILES", Cell: func(g *agentv1.AgentGrant) string {
		if len(g.GetProfiles()) == 0 {
			return "all"
		}
		return strings.Join(g.GetProfiles(), ",")
	}},
}

func newRevokeCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <agent> <org|app:NAME|env:APP/ENV|group:ID>",
		Short: "Withdraw a grant",
		Long: `Withdraw the grant to a target, whatever its profiles. Environments that
were assigned the agent under it keep the assignment, and their new jobs
wait until it is granted again or they change it.`,
		Args: flags.ExactArgs(2),
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
			g, err := parseGrant(cmd, c, args[1], nil)
			if err != nil {
				return err
			}
			if _, err := c.Agent().RevokeAgentUse(cmd.Context(), &agentv1.RevokeAgentUseRequest{AgentId: id, Grant: g}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "grant of agent %s to %s revoked\n", args[0], args[1])
			return nil
		},
	}
}

// parseGrant reads org, app:<name or id>, env:<app/env or id> or
// group:<grp- id>.
func parseGrant(cmd *cobra.Command, c sdkclient.AdmiralClient, target string, profiles []string) (*agentv1.AgentGrant, error) {
	g := &agentv1.AgentGrant{Profiles: profiles}
	kind, ref, _ := strings.Cut(target, ":")
	ctx := cmd.Context()
	switch kind {
	case "org":
		g.Target = &agentv1.AgentGrant_Organization_{Organization: &agentv1.AgentGrant_Organization{}}
	case "app":
		id, err := resolve.App(ctx, c.Application(), ref)
		if err != nil {
			return nil, err
		}
		g.Target = &agentv1.AgentGrant_ApplicationId{ApplicationId: id}
	case "env":
		app, env, err := flags.EnvTarget(cmd, "", ref)
		if err != nil {
			return nil, err
		}
		id, err := resolve.Environment(ctx, c.Environment(), c.Application(), app, env)
		if err != nil {
			return nil, err
		}
		g.Target = &agentv1.AgentGrant_EnvironmentId{EnvironmentId: id}
	case "group":
		g.Target = &agentv1.AgentGrant_GroupId{GroupId: ref}
	default:
		return nil, cmderr.Usage("grant target %q is not org, app:NAME, env:APP/ENV or group:ID", target)
	}
	return g, nil
}
