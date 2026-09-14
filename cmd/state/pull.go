package state

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

// state pull / push are temporarily stubbed in V2. The HTTP state endpoint
// is keyed by component UUID, but the standalone ComponentAPI that the CLI
// used for slug -> id resolution was removed in favor of change set
// workflows. Restoring slug-based access requires either a small server-side
// lookup RPC or reshaping the state HTTP endpoint to accept (app_id, slug,
// env_id). Tracked as a follow-up before V2 ship.

func newPullCmd(_ *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download Terraform state (deferred in V2)",
		Long:  `Temporarily disabled in V2. See follow-up for component-name-based state access.`,
		Args:  flags.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("state pull is temporarily disabled in V2 pending component-name-based lookup")
		},
	}
	return cmd
}
