package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newRemoveCmd(opts *client.Options) *cobra.Command {
	var (
		orphan bool
		ifRev  int32
		po     planOptions
	)

	cmd := &cobra.Command{
		Use:               "remove <changeset> <component>",
		ValidArgsFunction: complete.Positions(complete.Envs(opts), complete.ComponentsOf(opts, false)),
		Short:             "Remove a component",
		Long: `Remove a component. A component this changeset adds is dropped and its
name released; a live one is proposed for destruction. Nothing is destroyed
by this command.

With --orphan, a live component is proposed to stop being managed instead:
its resources stay where they are.`,
		Example: `  admiral changeset remove cs-7f2a1c9d0e3b users-db

  # Stop managing it, and leave its resources running
  admiral changeset remove cs-7f2a1c9d0e3b users-db --orphan`,
		Aliases: []string{"rm"},
		Args:    flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseChangeSet(args[0])
			if err != nil {
				return err
			}
			if err := componentName(args[1]); err != nil {
				return err
			}
			e := &changesetv1.Edit{
				Edit: &changesetv1.Edit_RemoveComponent{RemoveComponent: &changesetv1.RemoveComponent{Component: args[1]}},
			}
			if orphan {
				e.Edit = &changesetv1.Edit_OrphanComponent{OrphanComponent: &changesetv1.OrphanComponent{Component: args[1]}}
			}
			return edits(cmd, opts, ref, ifRevision(cmd, ifRev), po, e)
		},
	}

	cmd.Flags().BoolVar(&orphan, "orphan", false, "stop managing a live component without destroying its resources")
	revisionFlag(cmd, &ifRev)
	planFlags(cmd, &po)

	return cmd
}
