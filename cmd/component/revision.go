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

// newRevisionCmd is the history of one component: every set of bytes ever
// published under the name, by digest.
func newRevisionCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revision",
		Short: "Inspect a component's revisions",
		Long: `Inspect a component's revisions.

A revision is one published set of bytes, identified by digest. Tags point
at revisions; a revision may carry several tags or none. Deprecated
revisions stay listed, marked as such, because environments may still run
them.`,
		Aliases:       []string{"revisions", "rev"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newRevisionListCmd(opts),
		newRevisionGetCmd(opts),
	)

	return cmd
}

func newRevisionListCmd(opts *client.Options) *cobra.Command {
	var (
		status    string
		pageSize  int32
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list <name>",
		Short: "List a component's revisions",
		Example: `  admiral component revision list cloud-sql

  # Only what can still be adopted
  admiral component revision list cloud-sql --status published

  # Full digests
  admiral component revision list cloud-sql -o wide`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			var f string
			if status != "" {
				// The flag has already refused anything but the two words;
				// the server's enum names are their upper-case forms.
				var err error
				if f, err = filter.Eq("status", strings.ToUpper(status)); err != nil {
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
			resp, err := c.Registry().ListRevisions(cmd.Context(), &registryv1.ListRevisionsRequest{
				ComponentId: componentID, Filter: f, PageSize: pageSize, PageToken: pageToken,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintList(output.List{
				Kind:          "revisions",
				Items:         output.Messages(resp.Revisions),
				Name:          func(i int) string { return args[0] + "@" + resp.Revisions[i].Digest },
				NextPageToken: resp.NextPageToken,
			}, revisionTable.Render(p, resp.Revisions...))
		},
	}

	flags.Enum(cmd, &status, "status", "", "only revisions in this status", "published", "deprecated")
	cmd.Flags().Int32Var(&pageSize, "page-size", 50, "maximum number of results per page")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a previous response")

	return cmd
}

func newRevisionGetCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>:<tag> | <name>@<digest>",
		Short: "Get one revision, by tag or digest",
		Example: `  admiral component revision get cloud-sql:v1.2.0
  admiral component revision get cloud-sql@sha256:3f9a...

  # The contract, provenance and findings
  admiral component revision get cloud-sql:v1.2.0 -o yaml`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Refs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, ref, err := splitRef(args[0])
			if err != nil {
				return err
			}
			if ref == "" {
				return cmderr.Usage("%q names no revision; use <name>:<tag> or <name>@<digest>", args[0])
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
			resp, err := c.Registry().GetRevision(cmd.Context(), &registryv1.GetRevisionRequest{
				ComponentId: componentID, Reference: ref,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Revision, name+"@"+resp.Revision.Digest, revisionTable.Render(p, resp.Revision))
		},
	}

	return cmd
}
