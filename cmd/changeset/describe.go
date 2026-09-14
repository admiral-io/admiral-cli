package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <changeset-id>",
		Short:   "Show everything about a change set",
		Long:    "Render a change set's identity, its component entries and its variable entries.\n\ndescribe is a human view. Use 'changeset get -o json' for the raw record.",
		Example: "  admiral changeset describe cs-7f2a1",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: args[0]})
			if err != nil {
				return err
			}
			cs := resp.ChangeSet

			d := output.NewDescribe()
			d.Field("ID", changeSetID(cs))
			d.Field("Title", cs.Title)
			d.Field("Description", cs.Description)
			d.Field("Application", orID(cs.ApplicationName, cs.ApplicationId))
			d.Field("Environment", orID(cs.EnvironmentName, cs.EnvironmentId))
			d.Field("Status", output.FormatEnum(cs.Status))
			d.Field("Created", output.FormatDescribeTime(cs.CreatedAt))
			d.Field("Created By", output.FormatActor(cs.CreatedBy))
			d.Field("Updated", output.FormatDescribeTime(cs.UpdatedAt))

			entries := make([][]string, 0, len(cs.Entries))
			for _, e := range cs.Entries {
				mod := ""
				if e.CatalogItemId != nil {
					mod = orID(e.GetCatalogItemName(), *e.CatalogItemId)
				}
				entries = append(entries, []string{e.ComponentName, output.FormatEnum(e.ChangeType), mod, e.GetRef()})
			}
			d.Table("Entries", []string{"Component", "Change", "Module", "Ref"}, entries)

			vars := make([][]string, 0, len(cs.VariableEntries))
			for _, v := range cs.VariableEntries {
				action, value := "Set", ""
				switch {
				case v.Value == nil:
					action = "Remove"
				case v.Sensitive:
					value = "<redacted>"
				default:
					value = *v.Value
				}
				vars = append(vars, []string{v.Key, action, output.FormatEnumKebab(v.Type), output.Truncate(value, 40)})
			}
			d.Table("Variables", []string{"Key", "Action", "Type", "Value"}, vars)

			if cs.Status == changesetv1.ChangeSetStatus_CHANGE_SET_STATUS_OPEN {
				d.Hint("To plan this change set, run: admiral changeset plan " + changeSetID(cs))
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral changeset get "+changeSetID(cs))
		},
	}
	return cmd
}
