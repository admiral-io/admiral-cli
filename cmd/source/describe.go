package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <name>",
		Short:   "Show everything about a source",
		Long:    "Render a source's identity, its credential and the outcome of its last connectivity test.\n\ndescribe is a human view. Use 'source get -o json' for the raw record.",
		Example: "  admiral source describe platform-modules",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Source().GetSource(cmd.Context(), &sourcev1.GetSourceRequest{SourceId: id})
			if err != nil {
				return err
			}
			s := resp.Source

			d := output.NewDescribe()
			d.Field("Name", s.Name)
			d.Field("ID", s.Id)
			d.Field("Type", output.FormatEnumKebab(s.Type))
			d.Field("URL", s.Url)
			d.Field("Credential", credentialDisplay(s))
			d.Field("Description", s.Description)
			d.Fields("Labels", output.LabelLines(s.Labels))
			d.Field("Created", output.FormatDescribeTime(s.CreatedAt))
			d.Field("Created By", output.FormatActor(s.CreatedBy))
			d.Field("Updated", output.FormatDescribeTime(s.UpdatedAt))
			d.Section("Last Test", func(b *output.Block) {
				b.Field("Status", output.FormatEnum(s.LastTestStatus))
				b.Field("Tested", output.FormatDescribeTime(s.LastTestedAt))
				b.Field("Error", s.LastTestError)
			})
			d.Hint("To list published versions, run: admiral source versions " + s.Name)

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral source get "+s.Name)
		},
	}
	return cmd
}
