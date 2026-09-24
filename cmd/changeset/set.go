package changeset

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newSetCmd(opts *client.Options) *cobra.Command {
	var (
		to         string
		setStrings []string
		ifRev      int32
		po         planOptions
	)

	cmd := &cobra.Command{
		Use:   "set <change-set> <component>.<path>=<value>...",
		Short: "Set values, or move a component's pin",
		Long: `Set values, or move a component's pin. One command cuts one revision.

A value reads as YAML, as with helm --set: 3 is a number, true a bool,
"3" a string, null clears a chart default, !ref users-db.host references
another component's output. --set-string keeps the characters as a string.
A dot inside a key is written \. or the key is quoted: api.annotations."a.b"=x.

With --to, the one argument after the change set is a component, and its
pin moves to that tag or digest of the same registry component.`,
		Example: `  admiral changeset set cs-7f2a1c9d0e3b api.image.tag=v1.4.0 api.replicas=3

  # A value that must stay a string
  admiral changeset set cs-7f2a1c9d0e3b --set-string api.build=0042

  # Refuse if someone else edited it since revision 4
  admiral changeset set cs-7f2a1c9d0e3b api.replicas=3 --if-revision 4

  # Move the pin
  admiral changeset set cs-7f2a1c9d0e3b api --to v1.4.0`,
		Args: flags.RangeArgs(1, 1+maxEdits),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}

			if to != "" {
				if len(setStrings) > 0 {
					return cmderr.Usage("--to cannot be combined with --set-string")
				}
				if len(args) != 2 || strings.ContainsAny(args[1], ".=") {
					return cmderr.UsageHint("Move one pin per command: 'admiral changeset set cs-… api --to v1.4.0'.",
						"--to takes exactly one component")
				}
				if err := componentName(args[1]); err != nil {
					return err
				}
				return edits(cmd, opts, csID, ifRevision(cmd, ifRev), po, &changesetv1.Edit{
					Edit: &changesetv1.Edit_SetPin{SetPin: &changesetv1.SetPin{Component: args[1], Reference: to}},
				})
			}

			es, err := setEdits(args[1:], setStrings)
			if err != nil {
				return err
			}
			if len(es) == 0 {
				return cmderr.UsageHint("Give <component>.<path>=<value>, --set-string, or a component with --to.",
					"nothing to set")
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev), po, es...)
		},
	}

	cmd.Flags().StringVar(&to, "to", "", "move the component's pin to this tag or sha256: digest")
	cmd.Flags().StringArrayVar(&setStrings, "set-string", nil, "set <component>.<path>=<value> as a string, whatever it looks like (repeatable)")
	revisionFlag(cmd, &ifRev)
	planFlags(cmd, &po)

	return cmd
}
