// Package source provides CLI commands for managing sources.
package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
)

// SourceCmd is the parent command for source operations.
type SourceCmd struct {
	Cmd *cobra.Command
}

// NewSourceCmd creates the source command tree.
func NewSourceCmd(opts *client.Options) *SourceCmd {
	root := &SourceCmd{}

	cmd := &cobra.Command{
		Use:           "source",
		Short:         "Manage sources",
		Long:          `Manage sources -- external artifact locations (git repos, registries, etc.) that Admiral fetches from.`,
		Aliases:       []string{"src", "sources"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
		newTestCmd(opts),
		newSyncCmd(opts),
		newVersionsCmd(opts),
	)

	root.Cmd = cmd
	return root
}