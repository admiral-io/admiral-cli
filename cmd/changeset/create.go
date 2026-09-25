package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		title       string
		description string
		sets        []string
		setStrings  []string
		po          planOptions
	)

	cmd := &cobra.Command{
		Use:   "create <app>/<env>",
		Short: "Open a change set against an environment",
		Long: `Open a change set against an environment.

With --set or --set-string the edits cut revision 1 in the same request;
without, the change set has no revision until its first edit. --plan plans
revision 1 and waits for its prepare, as 'admiral changeset plan' does.`,
		Example: `  admiral changeset create shop/prod --title "Bump api to 1.4.0"

  # Open it with values already set
  admiral changeset create shop/prod --set api.image.tag=v1.4.0 --set-string api.build=0042`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := flags.EnvTarget(cmd, "", args[0])
			if err != nil {
				return err
			}
			if app == "" && !resolve.IsUUID(env) {
				return cmderr.UsageHint("Give the environment as app/env: 'admiral changeset create shop/prod'.",
					"no application specified")
			}
			es, err := setEdits(sets, setStrings)
			if err != nil {
				return err
			}
			if len(es) > maxEdits {
				return cmderr.Usage("%d edits in one command; the limit is %d", len(es), maxEdits)
			}
			if err := po.check(cmd); err != nil {
				return err
			}
			if po.plan && len(es) == 0 {
				return cmderr.UsageHint("Pass --set or --set-string, or plan after the first edit.",
					"--plan needs a revision, and a change set created without edits has none")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, env)
			if err != nil {
				return err
			}
			resp, err := c.ChangeSet().CreateChangeSet(cmd.Context(), &changesetv1.CreateChangeSetRequest{
				EnvironmentId: envID,
				Title:         title,
				Description:   description,
				Edits:         es,
				Plan:          po.plan,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			printWarnings(p.Err(), resp.Warnings, resp.Revision.GetViolations())
			output.Confirmed(p.Err(), "change set", resp.ChangeSet.Id, "created")
			if po.plan {
				return finishPrepare(cmd, opts, c.ChangeSet(), resp.ChangeSet.Id, resp.Prepare, po)
			}
			return p.PrintOne(resp.ChangeSet, resp.ChangeSet.Id, changeSetTable.Render(p, resp.ChangeSet))
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "a one-line title")
	cmd.Flags().StringVar(&description, "description", "", "what the change is for")
	setFlags(cmd, &sets, &setStrings)
	planFlags(cmd, &po)

	return cmd
}
