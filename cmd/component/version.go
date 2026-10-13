package component

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// newVersionCmd is the history of one component: every set of bytes ever
// published under the name, by digest.
func newVersionCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Inspect a component's versions",
		Long: `Inspect a component's versions.

A version is one published set of bytes, identified by digest. Tags point
at versions; a version may carry several tags or none. Deprecated and
revoked versions stay listed, marked as such, because environments may
still run them.`,
		Aliases:       []string{"versions", "ver"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newVersionListCmd(opts),
		newVersionGetCmd(opts),
	)

	return cmd
}

func newVersionListCmd(opts *client.Options) *cobra.Command {
	var (
		status string
		paging flags.PagingOptions
	)

	cmd := &cobra.Command{
		Use:   "list <name>",
		Short: "List a component's versions",
		Example: `  admiral component version list cloud-sql

  # Only what can still be adopted
  admiral component version list cloud-sql --status published

  # Full digests and version ids
  admiral component version list cloud-sql -o wide`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			var f string
			if status != "" {
				// The flag has already refused anything but the three words;
				// the server's enum names are their upper-case forms.
				var err error
				if f, err = filter.Eq("status", "STATUS_"+strings.ToUpper(status)); err != nil {
					return err
				}
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			componentID, err := resolve.Component(cmd.Context(), c.Registry(), args[0])
			if err != nil {
				return err
			}
			vers, next, err := flags.Pages(paging, func(token string) ([]*registryv1.Version, string, error) {
				resp, err := c.Registry().ListVersions(cmd.Context(), &registryv1.ListVersionsRequest{
					ComponentId: componentID, Filter: f, PageSize: paging.PageSize, PageToken: token,
				})
				if err != nil {
					return nil, "", err
				}
				return resp.Versions, resp.NextPageToken, nil
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "versions",
				Items:         output.Messages(vers),
				Name:          func(i int) string { return args[0] + "@" + vers[i].Digest },
				NextPageToken: next,
			}, versionTable.Render(p, vers...))
		},
	}

	flags.Enum(cmd, &status, "status", "", "only versions in this status", "published", "deprecated", "revoked")
	flags.Paging(cmd, &paging)

	return cmd
}

func newVersionGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>:<tag> | <name>@<digest|id>",
		Short: "Get one version, by tag, digest or id",
		Example: `  admiral component version get cloud-sql:v1.2.0
  admiral component version get cloud-sql@sha256:3f9a...
  admiral component version get cloud-sql@ver-7q2k9m4x8p1c

  # The contract, provenance and findings
  admiral component version get cloud-sql:v1.2.0 -o yaml`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Refs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, ref, err := splitRef(args[0])
			if err != nil {
				return err
			}
			if ref == "" {
				return cmderr.Usage("%q names no version; use <name>:<tag> or <name>@<digest>", args[0])
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			componentID, err := resolve.Component(cmd.Context(), c.Registry(), name)
			if err != nil {
				return err
			}
			resp, err := c.Registry().GetVersion(cmd.Context(), &registryv1.GetVersionRequest{
				ComponentId: componentID, Reference: ref,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Version, name+"@"+resp.Version.Digest, versionTable.Render(p, resp.Version))
		},
	}

	return cmd
}
