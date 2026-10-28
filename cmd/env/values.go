package env

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	"go.admiral.io/cli/internal/valuesfile"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

func newValuesCmd(opts *client.Options) *cobra.Command {
	var (
		appName   string
		effective bool
	)

	cmd := &cobra.Command{
		Use:   "values <name> <component>",
		Short: "Print the values a live component runs with",
		Long: `Print the values set on a component live in the environment, as YAML.

The output is a values file 'admiral changeset add --values' reads, so a
component's values can be carried to another environment.`,
		Example: `  admiral env values shop/dev api

  # Carry dev's values to a prod changeset
  admiral env values shop/dev api > api.yaml
  admiral changeset add shop/prod api:v1.4.0 --values api.yaml`,
		Args:              flags.ExactArgs(2),
		ValidArgsFunction: complete.Positions(complete.Envs(opts), complete.ComponentsOf(opts, false)),
		RunE: func(cmd *cobra.Command, args []string) error {
			appScope, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}
			comp := args[1]

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			envID, err := resolve.Environment(cmd.Context(), c.Environment(), c.Application(), appScope, name)
			if err != nil {
				return err
			}
			resp, err := c.Environment().GetEnvironmentComponentValues(cmd.Context(), &environmentv1.GetEnvironmentComponentValuesRequest{
				EnvironmentId: envID,
				Component:     comp,
				Effective:     effective,
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			if p.Format.IsMachine() {
				return p.PrintResource(resp, nil)
			}
			tree, err := valuesfile.DecodeJSON(resp.TreeJson)
			if err != nil {
				return err
			}
			body, err := valuesfile.Encode(tree)
			if err != nil {
				return err
			}
			var b bytes.Buffer
			fmt.Fprintf(&b, "# environment: %s\n# component: %s\n", args[0], comp)
			if la := resp.LastApply; la != nil {
				fmt.Fprintf(&b, "# applied by: %s %s\n", la.ChangeSetId, la.RevisionId)
			}
			if effective {
				b.WriteString("# effective: every value the component receives\n")
			}
			b.Write(body)
			_, err = p.Out().Write(b.Bytes())
			return err
		},
	}

	flags.App(cmd, &appName, opts)
	cmd.Flags().BoolVar(&effective, "effective", false, "print the whole tree the component receives, defaults included")
	// Hidden until the platform builds the effective tree; it refuses the flag today.
	_ = cmd.Flags().MarkHidden("effective")
	return cmd
}
