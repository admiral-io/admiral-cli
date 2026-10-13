package agent

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newProviderCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider",
		Aliases: []string{"providers"},
		Short:   "Manage the workload identity providers agents sign in through",
		Long: `A workload identity provider is an issuer whose tokens Admiral trusts: a
Kubernetes cluster, or any OIDC issuer. Its keys come from a JWKS URL, a
pasted key set, or enrollment by an agent running inside it.`,
	}
	cmd.AddCommand(newProviderListCmd(opts), newProviderGetCmd(opts), newProviderCreateCmd(opts),
		newProviderDeleteCmd(opts), newProviderSetKeysCmd(opts))
	return cmd
}

func newProviderListCmd(opts *client.Options) *cobra.Command {
	var paging flags.PagingOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workload identity providers",
		Args:  flags.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			ps, next, err := flags.Pages(paging, func(token string) ([]*agentv1.WorkloadIdentityProvider, string, error) {
				resp, err := c.WorkloadIdentityProvider().ListWorkloadIdentityProviders(cmd.Context(),
					&agentv1.ListWorkloadIdentityProvidersRequest{PageSize: paging.PageSize, PageToken: token})
				if err != nil {
					return nil, "", err
				}
				return resp.GetWorkloadIdentityProviders(), resp.GetNextPageToken(), nil
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind: "providers", Items: output.Messages(ps), NextPageToken: next,
				Name: func(i int) string { return ps[i].GetName() },
			}, providerTable.Render(p, ps...))
		},
	}
	flags.Paging(cmd, &paging)
	return cmd
}

func newProviderGetCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <provider>",
		Short: "Get a workload identity provider",
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			id, err := resolve.Provider(cmd.Context(), c.WorkloadIdentityProvider(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.WorkloadIdentityProvider().GetWorkloadIdentityProvider(cmd.Context(),
				&agentv1.GetWorkloadIdentityProviderRequest{ProviderId: id})
			if err != nil {
				return err
			}
			wip := resp.GetWorkloadIdentityProvider()
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(wip, wip.GetName(), providerTable.Render(p, wip))
		},
	}
}

func newProviderCreateCmd(opts *client.Options) *cobra.Command {
	var issuer, jwksURI, jwksFile string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a workload identity provider",
		Long: `Create a provider. With --jwks-uri or --jwks-file it is trusted at once
and --issuer is required. With neither it stays pending until an agent
enrolls its keys, and the issuer is taken from that agent's token.`,
		Example: `  # A kind cluster, enrolled by the agent running in it
  admiral agent provider create kind

  # GitHub Actions
  admiral agent provider create github --issuer https://token.actions.githubusercontent.com \
    --jwks-uri https://token.actions.githubusercontent.com/.well-known/jwks`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := keysFromFlags(cmd.InOrStdin(), jwksURI, jwksFile, false)
			if err != nil {
				return err
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			resp, err := c.WorkloadIdentityProvider().CreateWorkloadIdentityProvider(cmd.Context(),
				&agentv1.CreateWorkloadIdentityProviderRequest{WorkloadIdentityProvider: &agentv1.WorkloadIdentityProvider{
					Name: args[0], Issuer: issuer, Keys: keys,
				}})
			if err != nil {
				return err
			}
			wip := resp.GetWorkloadIdentityProvider()
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, providerTable.Render(p, wip))
		},
	}
	cmd.Flags().StringVar(&issuer, "issuer", "", "the iss every token must carry")
	cmd.Flags().StringVar(&jwksURI, "jwks-uri", "", "URL the verifying keys are fetched from")
	cmd.Flags().StringVar(&jwksFile, "jwks-file", "", "file holding a JSON Web Key Set (- for stdin)")
	cmd.MarkFlagsMutuallyExclusive("jwks-uri", "jwks-file")
	return cmd
}

func newProviderDeleteCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <provider>",
		Short: "Delete a workload identity provider",
		Long:  `Delete a provider. Refused while an agent signs in through it.`,
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			id, err := resolve.Provider(cmd.Context(), c.WorkloadIdentityProvider(), args[0])
			if err != nil {
				return err
			}
			if _, err := c.WorkloadIdentityProvider().DeleteWorkloadIdentityProvider(cmd.Context(),
				&agentv1.DeleteWorkloadIdentityProviderRequest{ProviderId: id}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "provider %s deleted\n", args[0])
			return nil
		},
	}
}

func newProviderSetKeysCmd(opts *client.Options) *cobra.Command {
	var (
		jwksURI, jwksFile   string
		enrollment, pending bool
	)
	cmd := &cobra.Command{
		Use:   "set-keys <provider>",
		Short: "Change where a provider's keys come from, or accept the keys an agent reported",
		Long: `Change where a provider's verifying keys come from. Exactly one of
--jwks-uri, --jwks-file, --enrollment or --accept-pending.

When the keys an enrolled agent reports differ from the trusted set, the
provider shows them as pending keys. --accept-pending trusts them.`,
		Example: `  admiral agent provider set-keys kind --accept-pending
  admiral agent provider set-keys private --jwks-file jwks.json`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &agentv1.SetWorkloadIdentityProviderKeysRequest{}
			if pending {
				req.Change = &agentv1.SetWorkloadIdentityProviderKeysRequest_AcceptPending_{
					AcceptPending: &agentv1.SetWorkloadIdentityProviderKeysRequest_AcceptPending{},
				}
			} else {
				keys, err := keysFromFlags(cmd.InOrStdin(), jwksURI, jwksFile, enrollment)
				if err != nil {
					return err
				}
				req.Change = &agentv1.SetWorkloadIdentityProviderKeysRequest_Keys{Keys: keys}
			}
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup
			if req.ProviderId, err = resolve.Provider(cmd.Context(), c.WorkloadIdentityProvider(), args[0]); err != nil {
				return err
			}
			resp, err := c.WorkloadIdentityProvider().SetWorkloadIdentityProviderKeys(cmd.Context(), req)
			if err != nil {
				return err
			}
			wip := resp.GetWorkloadIdentityProvider()
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, providerTable.Render(p, wip))
		},
	}
	cmd.Flags().StringVar(&jwksURI, "jwks-uri", "", "fetch the keys from this URL")
	cmd.Flags().StringVar(&jwksFile, "jwks-file", "", "trust the JSON Web Key Set in this file (- for stdin)")
	cmd.Flags().BoolVar(&enrollment, "enrollment", false, "let the agents inside keep the keys current")
	cmd.Flags().BoolVar(&pending, "accept-pending", false, "trust the keys an agent last reported")
	cmd.MarkFlagsMutuallyExclusive("jwks-uri", "jwks-file", "enrollment", "accept-pending")
	cmd.MarkFlagsOneRequired("jwks-uri", "jwks-file", "enrollment", "accept-pending")
	return cmd
}

// keysFromFlags returns nil when no source is given, which on create means
// enrollment.
func keysFromFlags(stdin io.Reader, jwksURI, jwksFile string, enrollment bool) (*agentv1.WorkloadIdentityProviderKeys, error) {
	switch {
	case jwksURI != "":
		return &agentv1.WorkloadIdentityProviderKeys{Source: &agentv1.WorkloadIdentityProviderKeys_JwksUri{JwksUri: jwksURI}}, nil
	case jwksFile != "":
		var (
			b   []byte
			err error
		)
		if jwksFile == "-" {
			b, err = io.ReadAll(stdin)
		} else {
			b, err = os.ReadFile(jwksFile)
		}
		if err != nil {
			return nil, cmderr.Usage("reading %s: %v", jwksFile, err)
		}
		return &agentv1.WorkloadIdentityProviderKeys{Source: &agentv1.WorkloadIdentityProviderKeys_Jwks{Jwks: string(b)}}, nil
	case enrollment:
		return &agentv1.WorkloadIdentityProviderKeys{Source: &agentv1.WorkloadIdentityProviderKeys_Enrollment_{
			Enrollment: &agentv1.WorkloadIdentityProviderKeys_Enrollment{},
		}}, nil
	}
	return nil, nil
}
