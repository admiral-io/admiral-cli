package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newTestCmd(opts *client.Options) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "test <source>",
		Short: "Test connectivity to a source",
		Long:  `Validate that the attached credential authenticates against the source URL. Persists outcome on the source.`,
		Args:  flags.ExactArgs(1),
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
			resp, err := c.Source().TestSource(cmd.Context(), &sourcev1.TestSourceRequest{SourceId: id})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, testResultTable.Render(p, resp))
		},
	}

	return cmd
}
