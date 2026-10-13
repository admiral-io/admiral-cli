package env

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		appName     string
		newName     string
		description string
		labelStrs   []string
		target      targetFlags
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an environment",
		Long: `Update an environment's mutable fields: name, description, labels, the
agents it deploys through, its namespace and the approvals an apply needs.

--agent replaces the list of agents. An agent that stays keeps its settings;
one that is dropped stops serving the environment.`,
		Example: `  # Update description
  admiral env update billing/staging --description "US East staging"

  # Add or update a label
  admiral env update billing/staging --label tier=staging

  # Remove a label (kubectl-style key-)
  admiral env update billing/staging --label legacy-

  # By name, scoped with --app
  admiral env update staging --app billing --label tier=staging

  # Move components to another namespace
  admiral env update billing/staging --namespace billing-staging

  # Deploy through two agents
  admiral env update billing/prod --agent prod-cluster --agent prod-iam

  # Applies need one approval
  admiral env update billing/prod --approvals 1

  # Update by ID
  admiral env update env-3k9x2m7qpl0a --description "..."`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}

			var paths []string
			if cmd.Flags().Changed("name") {
				paths = append(paths, "name")
			}
			if cmd.Flags().Changed("description") {
				paths = append(paths, "description")
			}
			if cmd.Flags().Changed("label") {
				paths = append(paths, "labels")
			}
			if err := target.check(cmd, false); err != nil {
				return err
			}
			targetChanged := cmd.Flags().Changed("agent") || cmd.Flags().Changed("namespace") || cmd.Flags().Changed("approvals")
			if len(paths) == 0 && !targetChanged {
				return cmderr.Usage("at least one of --name, --description, --label, --agent, --namespace or --approvals must be specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, name)
			if err != nil {
				return err
			}

			// Read-modify-write so untouched fields keep their values.
			current, err := c.Environment().GetEnvironment(cmd.Context(), &environmentv1.GetEnvironmentRequest{EnvironmentId: envID})
			if err != nil {
				return err
			}
			e := current.Environment

			if cmd.Flags().Changed("name") {
				e.Name = newName
			}
			if cmd.Flags().Changed("description") {
				e.Description = description
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(e.Labels, labelStrs)
				if err != nil {
					return err
				}
				e.Labels = labels
			}
			targetPaths, err := target.apply(cmd.Context(), cmd, c.Agent(), e)
			if err != nil {
				return err
			}
			paths = append(paths, targetPaths...)
			resp, err := c.Environment().UpdateEnvironment(cmd.Context(), &environmentv1.UpdateEnvironmentRequest{
				Environment: e,
				UpdateMask:  &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Environment, resp.Environment.Name, envTable.Render(p, resp.Environment))
		},
	}

	flags.App(cmd, &appName, opts)
	cmd.Flags().StringVar(&newName, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	target.register(cmd, "agent to deploy through, by name or agt- id (repeatable; replaces the list)",
		"the Kubernetes namespace components render into unless they name one (needs exactly one agent)")

	return cmd
}
