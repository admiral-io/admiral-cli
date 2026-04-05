package app

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
)

// AppCmd is the parent command for application operations.
type AppCmd struct {
	Cmd *cobra.Command
}

// NewAppCmd creates the app command tree.
func NewAppCmd(opts *client.Options) *AppCmd {
	root := &AppCmd{}

	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
		Long: `Manage applications and their lifecycle.

Most commands accept an app name as a positional argument or use --id for UUID lookup.`,
		Aliases:       []string{"application"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}

	cmd.AddCommand(
		newListCmd(opts),
		newCreateCmd(opts),
		newGetCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}
