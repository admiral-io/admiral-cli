package component

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newDeprecateCmd(opts *client.Options) *cobra.Command {
	var (
		digest string
		reason string
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "deprecate <name> --digest <digest> --reason <text>",
		Short: "Deprecate a revision",
		Long: `Deprecate a revision.

A deprecated revision refuses new adoption; environments already pinned to
it keep running and see the reason as a warning. Deprecation is by digest,
not tag, because what is being deprecated is the bytes, and a floating tag
may name different bytes by the time anyone reads the reason. It is not
reversible.`,
		Example: `  admiral component deprecate cloud-sql --digest sha256:3f9a... \
    --reason "CVE-2026-1234 in vendored submodule"`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			componentID, err := resolve.Component(cmd.Context(), c.Registry(), args[0])
			if err != nil {
				return err
			}

			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Deprecate %s@%s (not reversible)", args[0], shortDigest(digest))); err != nil {
				return err
			}

			resp, err := c.Registry().DeprecateRevision(cmd.Context(), &registryv1.DeprecateRevisionRequest{
				ComponentId: componentID, Digest: digest, Reason: reason,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Confirmed(p.Err(), "revision", args[0]+"@"+shortDigest(resp.Revision.Digest), "deprecated")
			return p.PrintOne(resp.Revision, args[0]+"@"+resp.Revision.Digest, revisionTable.Render(p, resp.Revision))
		},
	}

	cmd.Flags().StringVar(&digest, "digest", "", "the revision to deprecate, as sha256:<hex>")
	cmd.Flags().StringVar(&reason, "reason", "", "why; shown to anyone who tries to adopt it and to every environment running it")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	_ = cmd.MarkFlagRequired("digest")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}
