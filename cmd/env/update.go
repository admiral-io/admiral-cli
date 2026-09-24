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
		kube        kubernetesFlags
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an environment",
		Long: `Update an environment's mutable fields: name, description, labels, and
the Kubernetes namespace and whether apply creates it.`,
		Example: `  # Update description
  admiral env update billing/staging --description "US East staging"

  # Add or update a label
  admiral env update billing/staging --label tier=staging

  # Remove a label (kubectl-style key-)
  admiral env update billing/staging --label legacy-

  # By name, scoped with --app
  admiral env update staging --app billing --label tier=staging

  # Move workload components to another namespace
  admiral env update billing/staging --namespace billing-staging

  # Update by UUID
  admiral env update 550e8400-e29b-41d4-a716-446655440000 --description "..."`,
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
			var kt *environmentv1.KubernetesTarget
			kubePaths, err := kube.apply(cmd, &kt)
			if err != nil {
				return err
			}
			paths = append(paths, kubePaths...)
			if len(paths) == 0 {
				return cmderr.Usage("at least one of --name, --description, --label, --namespace or --create-namespaces must be specified")
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
			if len(kubePaths) > 0 {
				if _, err := kube.apply(cmd, &e.Kubernetes); err != nil {
					return err
				}
			}
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
	kube.register(cmd, "the Kubernetes namespace workload components go to")

	return cmd
}
