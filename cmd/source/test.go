package source

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newTestCmd(opts *client.Options) *cobra.Command {
	var srcID string

	cmd := &cobra.Command{
		Use:   "test [source]",
		Short: "Test connectivity to a source",
		Long:  `Validate that the attached credential authenticates against the source URL. Persists outcome on the source.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && srcID == "" {
				_ = cmd.Help()
				return fmt.Errorf("source name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveSourceID(cmd.Context(), c.Source(), nameArg, srcID)
			if err != nil {
				return err
			}
			resp, err := c.Source().TestSource(cmd.Context(), &sourcev1.TestSourceRequest{SourceId: id})
			if err != nil {
				return err
			}

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				output.Writeln(w, "STATUS\tERROR")
				errMsg := resp.Error
				if errMsg == "" {
					errMsg = "-"
				}
				output.Writef(w, "%s\t%s\n", testStatusFromResponse(resp.Status), errMsg)
			})
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	return cmd
}

func testStatusFromResponse(s sourcev1.SourceTestStatus) string {
	if v, ok := testStatusLabel[s]; ok {
		return v
	}
	return s.String()
}