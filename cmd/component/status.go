package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

// statusVerb is one of the three commands that set a status. Given a bare
// name a verb acts on the component, given a reference on one version;
// component is the zero value when the verb has no component form.
type statusVerb struct {
	use, verb, short, long, example, done string
	component                             registryv1.Component_Status
	version                               registryv1.Version_Status
}

var (
	deprecateVerb = statusVerb{
		use:   "deprecate",
		verb:  "Deprecate",
		short: "Deprecate a component or one of its versions",
		long: `Deprecate a component or one of its versions.

A deprecated component or version can still be adopted, with a warning, and
environments already running it see the reason too. A component may name a
successor to move to. To refuse new adoption of a version, revoke it. Undo
with restore.`,
		example: `  # The whole component, pointing at what replaces it
  admiral component deprecate cloud-sql --reason "use cloud-sql-v2" --successor cloud-sql-v2

  # One version
  admiral component deprecate cloud-sql:v1.2.0 --reason "use v1.3.0"
  admiral component deprecate cloud-sql@sha256:3f9a... --reason "use v1.3.0"`,
		done:      "deprecated",
		component: registryv1.Component_STATUS_DEPRECATED,
		version:   registryv1.Version_STATUS_DEPRECATED,
	}
	revokeVerb = statusVerb{
		use:   "revoke",
		verb:  "Revoke",
		short: "Revoke a version",
		long: `Revoke a version.

A revoked version refuses new adoption; environments already running it
keep running and see the reason as a warning. Undo with restore.`,
		example: `  admiral component revoke cloud-sql@sha256:3f9a... \
    --reason "CVE-2026-1234 in vendored submodule"`,
		done:    "revoked",
		version: registryv1.Version_STATUS_REVOKED,
	}
	restoreVerb = statusVerb{
		use:   "restore",
		verb:  "Restore",
		short: "Restore a deprecated component, or a deprecated or revoked version",
		long: `Restore a deprecated component to active, or a deprecated or revoked
version to published.

A version's reason is recorded, as it is for deprecate and revoke.`,
		example: `  admiral component restore cloud-sql
  admiral component restore cloud-sql@sha256:3f9a... --reason "false positive"`,
		done:      "restored",
		component: registryv1.Component_STATUS_ACTIVE,
		version:   registryv1.Version_STATUS_PUBLISHED,
	}
)

// newStatusCmd builds one status verb. A version's status is set by digest,
// not tag, because it belongs to the bytes and a floating tag may name
// different bytes by the time anyone reads the reason; a tag is resolved to
// its digest first, and the prompt shows which.
func newStatusCmd(opts *client.Options, v statusVerb) *cobra.Command {
	var (
		reason    string
		successor string
		force     bool
	)

	use := v.use + " <name>[:<tag>|@<digest>] --reason <text>"
	if v.component == registryv1.Component_STATUS_UNSPECIFIED {
		use = v.use + " <name>:<tag>|<name>@<digest> --reason <text>"
	}

	cmd := &cobra.Command{
		Use:               use,
		Short:             v.short,
		Long:              v.long,
		Example:           v.example,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Refs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, ref, err := splitRef(args[0])
			if err != nil {
				return err
			}
			switch {
			case ref == "" && v.component == registryv1.Component_STATUS_UNSPECIFIED:
				return cmderr.Usage("%q names no version; use <name>:<tag> or <name>@<digest>", args[0])
			case ref != "" && successor != "":
				return cmderr.Usage("--successor applies to a component, not a version")
			case reason == "" && (ref != "" || v.component == registryv1.Component_STATUS_DEPRECATED):
				return cmderr.Usage("--reason is required")
			}
			if err := input.RequireInteractiveOrForce(cmd, force); err != nil {
				return err
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
			p := output.NewPrinter(cmd, opts.OutputFormat)

			if ref == "" {
				return setComponentStatus(cmd, p, c, v, componentID, name, reason, successor, force)
			}

			digest, err := digestOf(cmd.Context(), c, componentID, ref)
			if err != nil {
				return err
			}
			if err := input.Confirm(cmd, force,
				fmt.Sprintf("%s %s@%s", v.verb, name, shortDigest(digest))); err != nil {
				return err
			}
			resp, err := c.Registry().SetVersionStatus(cmd.Context(), &registryv1.SetVersionStatusRequest{
				ComponentId: componentID, Digest: digest, Status: v.version, Reason: reason,
			})
			if err != nil {
				return err
			}
			output.Confirmed(p.Err(), "version", name+"@"+shortDigest(resp.Version.Digest), v.done)
			return p.PrintOne(resp.Version, name+"@"+resp.Version.Digest, versionTable.Render(p, resp.Version))
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "why; shown to anyone who adopts it and to every environment running it")
	if v.component == registryv1.Component_STATUS_DEPRECATED {
		cmd.Flags().StringVar(&successor, "successor", "", "the component to use instead, by name or ID")
		complete.Flag(cmd, "successor", complete.Components(opts))
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}

func setComponentStatus(cmd *cobra.Command, p *output.Printer, c sdkclient.AdmiralClient, v statusVerb, componentID, name, reason, successor string, force bool) error {
	var successorID string
	if successor != "" {
		id, err := resolve.Component(cmd.Context(), c.Registry(), successor)
		if err != nil {
			return err
		}
		successorID = id
	}
	if err := input.Confirm(cmd, force, fmt.Sprintf("%s %s", v.verb, name)); err != nil {
		return err
	}
	resp, err := c.Registry().SetComponentStatus(cmd.Context(), &registryv1.SetComponentStatusRequest{
		ComponentId: componentID, Status: v.component, Reason: reason, SuccessorComponentId: successorID,
	})
	if err != nil {
		return err
	}
	output.Confirmed(p.Err(), "component", resp.Component.Name, v.done)
	return p.PrintOne(resp.Component, resp.Component.Name, componentTable.Render(p, resp.Component))
}

// digestOf is the digest a reference names. A digest is its own answer; a
// tag or version id is looked up.
func digestOf(ctx context.Context, c sdkclient.AdmiralClient, componentID, ref string) (string, error) {
	if strings.HasPrefix(ref, "sha256:") {
		return ref, nil
	}
	resp, err := c.Registry().GetVersion(ctx, &registryv1.GetVersionRequest{ComponentId: componentID, Reference: ref})
	if err != nil {
		return "", err
	}
	return resp.Version.Digest, nil
}
