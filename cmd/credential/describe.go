package credential

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "describe <name>",
		Short:   "Show everything about a credential",
		Long:    "Render a credential's metadata. Secret material is never shown.\n\ndescribe is a human view. Use 'credential get -o json' for the raw record.",
		Example: "  admiral credential describe ci-deploy",
		Aliases: []string{"desc"},
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Credential(cmd.Context(), c.Credential(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{CredentialId: id})
			if err != nil {
				return err
			}
			cr := resp.Credential

			d := output.NewDescribe()
			d.Field("Name", cr.Name)
			d.Field("ID", cr.Id)
			d.Field("Type", output.FormatEnumKebab(cr.Type))
			d.Field("Description", cr.Description)
			d.Fields("Labels", output.LabelLines(cr.Labels))
			d.Field("Created", output.FormatDescribeTime(cr.CreatedAt))
			d.Field("Created By", output.FormatActor(cr.CreatedBy))
			d.Field("Updated", output.FormatDescribeTime(cr.UpdatedAt))

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral credential get "+cr.Name)
		},
	}
	return cmd
}
