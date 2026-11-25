package changeset

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newRebaseCmd(opts *client.Options) *cobra.Command {
	var (
		keepChangeSet, keepEnv []string
		po                     planOptions
	)

	cmd := &cobra.Command{
		Use:               "rebase <changeset>",
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		Short:             "Re-apply a changeset that fell behind onto its environment as it is now",
		Long: `Re-apply a changeset that fell behind onto its environment as it is now.

A changeset falls behind whenever its environment moves, for example when
another changeset applies, and it cannot apply until it is rebased. The
rebase cuts one new revision.

A setting both this changeset and the environment changed is a conflict,
and each one needs a resolution: --keep-changeset keeps this changeset's
side, --keep-environment the environment's. Name a value as component.path,
as --set does, or a bare component for its version, or for a component the
environment no longer has. Without every resolution the rebase lists the
conflicts and the flags that settle them. Resolving any conflict dismisses
the changeset's approvals.`,
		Example: `  admiral changeset rebase cs-7f2a1c9d0e3b
  admiral changeset rebase cs-7f2a1c9d0e3b --plan

  # Keep this changeset's image tag and the environment's version of web
  admiral changeset rebase cs-7f2a1c9d0e3b --keep-changeset api.image.tag --keep-environment web`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}

			wanted, err := parseKeeps(keepChangeSet, keepEnv)
			if err != nil {
				return err
			}

			if err := po.check(cmd); err != nil {
				return err
			}

			c, csID, err := dial(cmd, opts, ref)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			got, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
			if err != nil {
				return err
			}

			if !got.GetChangeSet().GetBehind() {
				output.Writef(cmd.ErrOrStderr(), "changeset %q is not behind its environment; nothing to rebase\n", csID)
				return nil
			}

			resolutions, err := resolveConflicts(csID, got.GetConflicts(), wanted)
			if err != nil {
				return err
			}

			head := got.GetChangeSet().GetHeadRevision()

			resp, err := c.ChangeSet().RebaseChangeSet(cmd.Context(), &changesetv1.RebaseChangeSetRequest{
				ChangeSetId: csID,
				IfRevision:  &head,
				Resolutions: resolutions,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			printWarnings(p.Err(), resp.Warnings, resp.Revision.GetViolations())
			output.Confirmed(p.Err(), "changeset", csID, "rebased as revision "+strconv.Itoa(int(resp.Revision.GetNumber())))

			if po.plan {
				return planRevision(cmd, opts, c.Run(), resp.ChangeSet, resp.Revision, po)
			}

			return p.PrintOne(resp.Revision, revisionName(csID, resp.Revision), revisionTable.Render(p, resp.Revision))
		},
	}

	cmd.Flags().StringArrayVar(&keepChangeSet, "keep-changeset", nil, "resolve a conflict with this changeset's side: component.path, or component for its version (repeatable)")
	cmd.Flags().StringArrayVar(&keepEnv, "keep-environment", nil, "resolve a conflict with the environment's side: component.path, or component for its version or removal (repeatable)")
	planFlags(cmd, &po)

	return cmd
}

// keep is one --keep-changeset or --keep-environment.
type keep struct {
	flag      string
	component string
	path      []string
	side      changesetv1.Resolution_Keep
}

func (k keep) target() string { return strings.Join(append([]string{k.component}, k.path...), ".") }

func parseKeeps(changeSet, env []string) ([]keep, error) {
	var out []keep

	add := func(flag string, side changesetv1.Resolution_Keep, vals []string) error {
		for _, v := range vals {
			comp, path, err := valuesfile.ParseTarget(v)
			if err != nil {
				return cmderr.Usage("--%s: %v", flag, err)
			}

			if err := componentName(comp); err != nil {
				return err
			}

			k := keep{flag: flag, component: comp, path: path, side: side}
			for _, o := range out {
				if o.component == k.component && slices.Equal(o.path, k.path) {
					return cmderr.Usage("%s is resolved twice, by --%s and --%s", k.target(), o.flag, k.flag)
				}
			}

			out = append(out, k)
		}

		return nil
	}
	if err := add("keep-changeset", changesetv1.Resolution_KEEP_CHANGE_SET, changeSet); err != nil {
		return nil, err
	}

	if err := add("keep-environment", changesetv1.Resolution_KEEP_ENVIRONMENT, env); err != nil {
		return nil, err
	}

	return out, nil
}

func conflictPath(c *changesetv1.Conflict) []string {
	keys := make([]string, 0, len(c.GetPath()))
	for _, s := range c.GetPath() {
		keys = append(keys, s.GetKey())
	}

	return keys
}

// matches is whether k names c: a path names a value conflict, a bare
// component its version or removal.
func (k keep) matches(c *changesetv1.Conflict) bool {
	if k.component != c.GetComponent() {
		return false
	}

	if c.GetKind() == changesetv1.Conflict_KIND_VALUE {
		return len(k.path) > 0 && slices.Equal(k.path, conflictPath(c))
	}

	return len(k.path) == 0
}

// resolveConflicts pairs every conflict with a flag. A flag naming no conflict, or a
// conflict no flag names, is refused here with the list and the flags that
// settle it, so the server is never asked to guess.
func resolveConflicts(csID string, conflicts []*changesetv1.Conflict, wanted []keep) ([]*changesetv1.Resolution, error) {
	var out []*changesetv1.Resolution

	used := make([]bool, len(wanted))

	var open []*changesetv1.Conflict

	for _, c := range conflicts {
		i := slices.IndexFunc(wanted, func(k keep) bool { return k.matches(c) })
		if i < 0 {
			open = append(open, c)
			continue
		}

		used[i] = true

		k := wanted[i]
		if c.GetKind() == changesetv1.Conflict_KIND_REMOVED && k.side != changesetv1.Resolution_KEEP_ENVIRONMENT {
			return nil, cmderr.Usage("%s was removed from the environment; only --keep-environment %s resolves it", k.component, k.component)
		}

		out = append(out, &changesetv1.Resolution{Component: c.GetComponent(), Kind: c.GetKind(), Path: c.GetPath(), Keep: k.side})
	}

	for i, k := range wanted {
		if !used[i] {
			return nil, cmderr.UsageHint(conflictList(csID, conflicts), "--%s %s names no conflict", k.flag, k.target())
		}
	}

	if len(open) > 0 {
		return nil, cmderr.UsageHint(conflictList(csID, open),
			"%s has %d conflict(s) with its environment; resolve each with --keep-changeset or --keep-environment", csID, len(open))
	}

	return out, nil
}

// conflictList says what each conflict is and the flags that settle it.
func conflictList(csID string, conflicts []*changesetv1.Conflict) string {
	if len(conflicts) == 0 {
		return "This changeset has no conflicts; rebase it without --keep flags: 'admiral changeset rebase " + csID + "'."
	}

	var b strings.Builder
	b.WriteString("Conflicts:")

	for _, c := range conflicts {
		target := strings.Join(append([]string{c.GetComponent()}, conflictPath(c)...), ".")
		switch c.GetKind() {
		case changesetv1.Conflict_KIND_VALUE:
			fmt.Fprintf(&b, "\n  %s: changeset %s, environment %s", target, orAbsent(c.GetChangeSetValueJson()), orAbsent(c.GetEnvironmentValueJson()))
			fmt.Fprintf(&b, "\n    --keep-changeset %s  or  --keep-environment %s", target, target)
		case changesetv1.Conflict_KIND_VERSION:
			fmt.Fprintf(&b, "\n  %s version: changeset %s, environment %s", target,
				cmp.Or(shortDigest(c.GetChangeSetVersion().GetDigest()), "(none)"), cmp.Or(shortDigest(c.GetEnvironmentVersion().GetDigest()), "(none)"))
			fmt.Fprintf(&b, "\n    --keep-changeset %s  or  --keep-environment %s", target, target)
		case changesetv1.Conflict_KIND_REMOVED:
			fmt.Fprintf(&b, "\n  %s: the environment no longer has it", target)
			fmt.Fprintf(&b, "\n    --keep-environment %s", target)
		}

		if id := c.GetEnvironmentChangeSetId(); id != "" {
			fmt.Fprintf(&b, "  (environment side from %s)", id)
		}
	}

	return b.String()
}

func orAbsent(js string) string {
	if js == "" {
		return "(absent)"
	}

	return js
}
