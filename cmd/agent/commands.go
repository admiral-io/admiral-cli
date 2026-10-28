package agent

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
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
		Short: "Get an agent",
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
			return p.PrintOne(a, a.GetName(), agentTable.Render(p, a))
		},
	}
}

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "update <agent>",
		Short: "Rename an agent",
		Long: `Rename an agent. Its id, service account, grants and the environments
assigned it stay as they are, and the installed agent keeps running: it
signs in by its service account, not its name.`,
		Example: `  admiral agent update dev --name dev-east`,
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
			resp, err := c.Agent().UpdateAgent(cmd.Context(), &agentv1.UpdateAgentRequest{
				Agent:      &agentv1.Agent{Id: id, Name: name},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, agentTable.Render(p, resp.GetAgent()))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "the agent's new name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

var agentKinds = map[string]agentv1.Agent_Kind{
	"kubernetes": agentv1.Agent_KIND_KUBERNETES,
	"infra":      agentv1.Agent_KIND_INFRA,
}

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		kind   string
		grants []string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent and its one-time enrollment key",
		Long: `Create an agent. Admiral makes the service account it runs as, and
returns a single-use enrollment key. Install the agent with the key where it
runs; on its first start it enrolls, and Admiral records the issuer and
subject it signs in with from then on.

The agent is installed with Helm into its own namespace, admiral-system,
never one it deploys into. The command prints the install.

The tenant's first agent is granted to the whole organization unless
--grant says otherwise; later ones serve nobody until granted.`,
		Example: `  admiral agent create dev --kind kubernetes

  # Granted to one application only, the checkout application
  admiral agent create prod --kind kubernetes --grant app:checkout`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			req := &agentv1.CreateAgentRequest{Agent: &agentv1.Agent{Name: args[0], Kind: agentKinds[kind]}}
			for _, g := range grants {
				grant, err := parseGrant(cmd, c, g)
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
	complete.Flag(cmd, "grant", complete.Scopes(opts))
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

// agentCharts are the charts each kind of agent installs from, named for
// the agent's image. The OCI reference needs no helm repo add.
var agentCharts = map[agentv1.Agent_Kind]string{
	agentv1.Agent_KIND_KUBERNETES: "admiral-k8s-agent",
	agentv1.Agent_KIND_INFRA:      "admiral-infra-agent",
}

// writeEnrollment prints the agent's id, its key with the expiry the server
// set, and the Helm install that enrolls it.
func writeEnrollment(w io.Writer, a *agentv1.Agent, key *agentv1.EnrollmentKey) {
	fmt.Fprintf(w, "\nAgent %s created.\n", a.GetId())
	fmt.Fprintf(w, "\nEnrollment key (single use, expires %s):\n%s\n",
		output.FormatDescribeTime(key.GetExpiresAt()), key.GetSecret())
	chart, ok := agentCharts[a.GetKind()]
	if !ok {
		return
	}
	// The key goes in a Secret, not a --set, so it never lands in the
	// release's stored values.
	// The release name holds the chart's name, so the chart uses it as is
	// rather than appending its own: the Kubernetes service account, and so
	// the subject the agent enrolls with, is <chart>-<name>.
	release := chart + "-" + a.GetName()
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
`, secret, key.GetSecret(), release, "oci://ghcr.io/admiral-io/admiral-helm/"+chart, a.GetServiceAccountId(), secret)
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
	return &cobra.Command{
		Use:               "grant <agent> <org|app:NAME|env:APP/ENV|group:ID>",
		ValidArgsFunction: complete.Positions(nil, complete.Scopes(opts)),
		Short:             "Let the organization, an application, an environment or a team use an agent",
		Long: `Grant use of an agent: every environment of the target may be assigned
it. The target is org, an application (app:NAME), one environment
(env:APP/ENV) or a group (group:ID). 'admiral agent get' lists the grants.`,
		Example: `  # Every environment in the organization may use the agent named dev
  admiral agent grant dev org

  # Only the checkout application's environments may use the agent named prod
  admiral agent grant prod app:checkout

  # Only checkout's staging environment
  admiral agent grant prod env:checkout/staging`,
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
			g, err := parseGrant(cmd, c, args[1])
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
}

func newRevokeCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:               "revoke <agent> <org|app:NAME|env:APP/ENV|group:ID>",
		ValidArgsFunction: complete.Positions(nil, complete.Scopes(opts)),
		Short:             "Withdraw a grant",
		Long: `Withdraw the grant to a target. Environments that were assigned the agent
under it keep the assignment, and their new jobs wait until it is granted
again or they change it.`,
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
			g, err := parseGrant(cmd, c, args[1])
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
func parseGrant(cmd *cobra.Command, c sdkclient.AdmiralClient, target string) (*agentv1.AgentGrant, error) {
	g := &agentv1.AgentGrant{}
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
