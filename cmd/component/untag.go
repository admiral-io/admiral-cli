package component

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newUntagCmd(opts *client.Options) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "untag <name>:<tag>",
		Short: "Remove a floating tag",
		Long: `Remove a floating tag.

A tag is a name over a revision, not the revision: removing it changes
nothing that is pinned, and the revision stays in the history, readable by
digest. A semver tag (v1.2.0) is immutable and cannot be removed; that is
what makes it a release.`,
		Example: `  admiral component untag cloud-sql:dev
  admiral component untag cloud-sql:sha-3f9a2c1 --force`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tag, err := splitRef(args[0])
			if err != nil {
				return err
			}
			if tag == "" {
				return cmderr.Usage("%q names no tag; use <name>:<tag>", args[0])
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

			if err := input.Confirm(cmd, force, fmt.Sprintf("Remove tag %s:%s", name, tag)); err != nil {
				return err
			}

			if _, err := c.Registry().DeleteTag(cmd.Context(), &registryv1.DeleteTagRequest{
				ComponentId: componentID, Name: tag,
			}); err != nil {
				return err
			}

			output.Confirmed(cmd.ErrOrStderr(), "tag", name+":"+tag, "removed")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
