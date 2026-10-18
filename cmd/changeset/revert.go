package changeset

import (
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
		po          planOptions
	)

	cmd := &cobra.Command{
		Use:   "revert <changeset>",
		Short: "Open a changeset that undoes an applied one",
		Long: `Open a changeset that returns every component the given changeset's apply
changed to its state before that apply. It is planned, approved and
applied like any other changeset, and may apply while a partial apply of
the reverted changeset holds the environment.

The changeset is named by its ID: an environment names only its open
changeset, which is never the one being reverted.`,
		Example: `  admiral changeset revert cs-7f2a1c9d0e3b

  # Open it and plan it
  admiral changeset revert cs-7f2a1c9d0e3b --plan`,
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
			})
			if err != nil {
				return err
			}
			cs := resp.GetChangeSet()

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Confirmed(p.Err(), "changeset", cs.GetId(), "opened to revert "+args[0])
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

	cmd.Flags().StringVar(&title, "title", "", `a one-line title (default "Revert" and the reverted changeset's title)`)
	cmd.Flags().StringVar(&description, "description", "", "what the revert is for")
	planFlags(cmd, &po)

	return cmd
}
