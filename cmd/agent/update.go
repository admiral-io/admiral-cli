package agent

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		newName     string
		description string
		labelStrs   []string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an agent",
		Long:  `Update an agent's mutable fields: name, description, labels.`,
		Example: `  # Update the description
  admiral agent update prod-agent --description "AWS production agent"

  # Add or update labels
  admiral agent update prod-agent --label team=platform --label cloud=aws

  # Remove a label
  admiral agent update prod-agent --label cloud-

  # Update by UUID
  admiral agent update <uuid> --description "AWS production agent"`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if len(paths) == 0 {
				return cmderr.Usage("at least one of --name, --description, or --label must be specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Agent(cmd.Context(), c.Agent(), args[0])
			if err != nil {
				return err
			}

			// Read-modify-write so untouched fields keep their values.
			current, err := c.Agent().GetAgent(cmd.Context(), &agentv1.GetAgentRequest{AgentId: id})
			if err != nil {
				return err
			}
			r := current.Agent

			if cmd.Flags().Changed("name") {
				r.Name = newName
			}
			if cmd.Flags().Changed("description") {
				r.Description = description
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(r.Labels, labelStrs)
				if err != nil {
					return err
				}
				r.Labels = labels
			}

			resp, err := c.Agent().UpdateAgent(cmd.Context(), &agentv1.UpdateAgentRequest{
				Agent:      r,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Agent, resp.Agent.Name, agentTable.Render(p, resp.Agent))
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")

	return cmd
}
