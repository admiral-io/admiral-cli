package changeset

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newRevertCmd(opts *client.Options) *cobra.Command {
	var (
		title       string
		description string
		components  []string
		po          planOptions
	)

	cmd := &cobra.Command{
		Use:   "revert <changeset>",
		Short: "Undo a changeset",
		Long: `Return what a changeset changed to its state before it.

While a partial apply of the changeset holds its environment, the revert is
its next revision: every component in it, or only those --component names,
goes back to its content from before the changeset. When that revision
lands, the changeset reads as reverted, or as applied with what it kept.

Once the changeset has applied, the revert opens a new changeset that
undoes all of it. When that one lands, the original reads as reverted.

Either way the revert is planned, approved and applied like any revision.
The changeset is named by its ID.`,
		Example: `  admiral changeset revert cs-7f2a1c9d0e3b

  # Undo only the component that failed, keep the rest, and plan it
  admiral changeset revert cs-7f2a1c9d0e3b --component api --plan`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !changeSetIDPattern.MatchString(args[0]) {
				return cmderr.UsageHint("Name the changeset to revert by its ID, as 'admiral changeset list <app>/<env>' shows it.",
					"invalid changeset %q: expected cs-xxxxxxxxxxxx", args[0])
			}
			if err := po.check(cmd); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().RevertChangeSet(cmd.Context(), &changesetv1.RevertChangeSetRequest{
				ChangeSetId: args[0],
				ChangeSet:   &changesetv1.ChangeSet{Title: title, Description: description},
				Components:  components,
			})
			if err != nil {
				return err
			}
			cs := resp.GetChangeSet()

			p := output.NewPrinter(cmd, opts.OutputFormat)
			if cs.GetId() == args[0] {
				output.Confirmed(p.Err(), "changeset", cs.GetId(), fmt.Sprintf("revision %d reverts it", cs.GetHeadRevision()))
			} else {
				output.Confirmed(p.Err(), "changeset", cs.GetId(), "opened to revert "+args[0])
			}
			if !po.plan {
				return p.PrintOne(cs, cs.GetId(), changeSetTable.Render(p, cs))
			}
			got, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: cs.GetId()})
			if err != nil {
				return err
			}
			return planRevision(cmd, opts, c.Run(), got.GetChangeSet(), got.GetHead(), po)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", `a new changeset's title (default "Revert" and the reverted changeset's title)`)
	cmd.Flags().StringVar(&description, "description", "", "what a new changeset's revert is for")
	cmd.Flags().StringArrayVar(&components, "component", nil, "a component to return, for a changeset holding its environment (repeatable; default all)")
	planFlags(cmd, &po)

	return cmd
}
