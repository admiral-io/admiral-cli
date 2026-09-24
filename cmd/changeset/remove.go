package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newRemoveCmd(opts *client.Options) *cobra.Command {
	var (
		ifRev int32
		po    planOptions
	)

	cmd := &cobra.Command{
		Use:   "remove <change-set> <component>",
		Short: "Remove a component",
		Long: `Remove a component. A component this change set adds is dropped and its
name released; a live one is proposed for destruction. Nothing is destroyed
by this command.`,
		Example: `  admiral changeset remove cs-7f2a1c9d0e3b users-db`,
		Aliases: []string{"rm"},
		Args:    flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			if err := componentName(args[1]); err != nil {
				return err
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev), po, &changesetv1.Edit{
				Edit: &changesetv1.Edit_RemoveComponent{RemoveComponent: &changesetv1.RemoveComponent{Component: args[1]}},
			})
		},
	}

	revisionFlag(cmd, &ifRev)
	planFlags(cmd, &po)

	return cmd
}
