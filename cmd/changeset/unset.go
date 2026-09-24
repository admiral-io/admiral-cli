package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newUnsetCmd(opts *client.Options) *cobra.Command {
	var ifRev int32

	cmd := &cobra.Command{
		Use:   "unset <change-set> <component>.<path>...",
		Short: "Remove values, so the defaults apply",
		Long: `Remove values, so the defaults apply. One command cuts one revision.

To clear a default instead, set the path to null.`,
		Example: `  admiral changeset unset cs-7f2a1c9d0e3b api.image.tag api.replicas`,
		Args:    flags.RangeArgs(2, 1+maxEdits),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			es := make([]*changesetv1.Edit, 0, len(args)-1)
			for _, s := range args[1:] {
				comp, path, err := valuesfile.ParsePath(s)
				if err != nil {
					return cmderr.Usage("%v", err)
				}
				if err := componentName(comp); err != nil {
					return err
				}
				es = append(es, &changesetv1.Edit{Edit: &changesetv1.Edit_UnsetValue{UnsetValue: &changesetv1.UnsetValue{
					Component: comp,
					Path:      pathSegments(path),
				}}})
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev), es...)
		},
	}

	revisionFlag(cmd, &ifRev)

	return cmd
}
