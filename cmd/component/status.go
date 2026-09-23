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

// statusVerb is one of the three commands that set a revision's status.
type statusVerb struct {
	use, verb, short, long, example, done string
	status                                registryv1.RevisionStatus
}

var (
	deprecateVerb = statusVerb{
		use:   "deprecate",
		verb:  "Deprecate",
		short: "Deprecate a revision",
		long: `Deprecate a revision.

A deprecated revision can still be adopted, with a warning, and environments
already running it see the reason too. To refuse new adoption, revoke it.
Undo with restore.`,
		example: `  admiral component deprecate cloud-sql --digest sha256:3f9a... --reason "use v2"`,
		done:    "deprecated",
		status:  registryv1.RevisionStatus_DEPRECATED,
	}
	revokeVerb = statusVerb{
		use:   "revoke",
		verb:  "Revoke",
		short: "Revoke a revision",
		long: `Revoke a revision.

A revoked revision refuses new adoption; environments already running it
keep running and see the reason as a warning. Undo with restore.`,
		example: `  admiral component revoke cloud-sql --digest sha256:3f9a... \
    --reason "CVE-2026-1234 in vendored submodule"`,
		done:   "revoked",
		status: registryv1.RevisionStatus_REVOKED,
	}
	restoreVerb = statusVerb{
		use:   "restore",
		verb:  "Restore",
		short: "Restore a deprecated or revoked revision",
		long: `Restore a deprecated or revoked revision to published.

The reason is recorded, as it is for deprecate and revoke.`,
		example: `  admiral component restore cloud-sql --digest sha256:3f9a... --reason "false positive"`,
		done:    "restored",
		status:  registryv1.RevisionStatus_PUBLISHED,
	}
)

// newStatusCmd builds one status verb. Status is set by digest, not tag,
// because it belongs to the bytes and a floating tag may name different
// bytes by the time anyone reads the reason.
func newStatusCmd(opts *client.Options, v statusVerb) *cobra.Command {
	var (
		digest string
		reason string
		force  bool
	)

	cmd := &cobra.Command{
		Use:               v.use + " <name> --digest <digest> --reason <text>",
		Short:             v.short,
		Long:              v.long,
		Example:           v.example,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Components(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.RequireInteractiveOrForce(cmd, force); err != nil {
				return err
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

			if err := input.Confirm(cmd, force,
				fmt.Sprintf("%s %s@%s", v.verb, args[0], shortDigest(digest))); err != nil {
				return err
			}

			resp, err := c.Registry().SetRevisionStatus(cmd.Context(), &registryv1.SetRevisionStatusRequest{
				ComponentId: componentID, Digest: digest, Status: v.status, Reason: reason,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Confirmed(p.Err(), "revision", args[0]+"@"+shortDigest(resp.Revision.Digest), v.done)
			return p.PrintOne(resp.Revision, args[0]+"@"+resp.Revision.Digest, revisionTable.Render(p, resp.Revision))
		},
	}

	cmd.Flags().StringVar(&digest, "digest", "", "the revision, as sha256:<hex>")
	cmd.Flags().StringVar(&reason, "reason", "", "why; shown to anyone who adopts it and to every environment running it")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	_ = cmd.MarkFlagRequired("digest")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}
