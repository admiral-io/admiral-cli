package agent

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
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
		Short: "Get an agent, with what it last reported",
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
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.GetAgent(), resp.GetAgent().GetName(), agentTable.Render(p, resp.GetAgent()))
		},
	}
}

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		provider, subject, kind string
		grants                  []string
		profiles                []string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent",
		Long: `Create an agent that signs in as --subject at --provider. For a
Kubernetes agent the subject is its service account,
system:serviceaccount:<namespace>:<name>.

When the provider has no keys yet, the answer carries a single-use
enrollment key: give it to the agent, which enrolls the provider's keys from
inside the cluster.

The tenant's first agent is granted to the whole organization unless
--grant says otherwise; later ones serve nobody until granted.`,
		Example: `  # A Kubernetes agent in a kind cluster
  admiral agent create kind --provider kind \
    --subject system:serviceaccount:admiral:admiral-agent

  # Granted to one application only
  admiral agent create prod --provider gke --subject system:serviceaccount:admiral:agent --grant app:shop`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			providerID, err := resolve.Provider(cmd.Context(), c.WorkloadIdentityProvider(), provider)
			if err != nil {
				return err
			}
			k := agentv1.Agent_KIND_KUBERNETES
			if kind == "terraform" {
				k = agentv1.Agent_KIND_TERRAFORM
			}
			req := &agentv1.CreateAgentRequest{Agent: &agentv1.Agent{
				Name: args[0], Kind: k,
				Identity: &agentv1.AgentIdentity{Method: &agentv1.AgentIdentity_Workload{Workload: &agentv1.WorkloadIdentity{
					ProviderId: providerID, Subject: subject,
				}}},
			}}
			for _, g := range grants {
				grant, err := parseGrant(cmd, c, g, profiles)
				if err != nil {
					return err
				}
				req.Grants = append(req.Grants, grant)
			}
			resp, err := c.Agent().CreateAgent(cmd.Context(), req)
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if err := p.PrintResource(resp, agentTable.Render(p, resp.GetAgent())); err != nil {
				return err
			}
			if key := resp.GetEnrollmentKey(); key != nil && opts.OutputFormat.IsTable() {
				fmt.Fprintf(p.Out(), "\nEnrollment key (single use, expires %s):\n%s\n",
					output.FormatDescribeTime(key.GetExpiresAt()), key.GetSecret())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "workload identity provider, by name or ID")
	cmd.Flags().StringVar(&subject, "subject", "", "the subject the provider's tokens name")
	flags.Enum(cmd, &kind, "kind", "kubernetes", "what the agent runs", "kubernetes", "terraform")
	cmd.Flags().StringArrayVar(&grants, "grant", nil, "grant use to org, app:<name>, env:<app/env> or group:<id> (repeatable)")
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "profiles the grants cover (repeatable; none covers all)")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("subject")
	return cmd
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
		Long: `Issue a single-use key the agent enrolls its provider's keys with. It
expires within an hour and replaces any key issued before. Only for a
provider that is still pending.`,
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
