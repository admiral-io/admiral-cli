package state

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

func newPushCmd(_ *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload Terraform state (deferred in V2)",
		Long:  `Temporarily disabled in V2. See follow-up for component-name-based state access.`,
		Args:  flags.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("state push is temporarily disabled in V2 pending component-name-based lookup")
		},
	}
	return cmd
}
