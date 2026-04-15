package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newCreateHTTPCmd(opts *client.Options) *cobra.Command {
	var flags commonCreateFlags

	cmd := &cobra.Command{
		Use:   "http <name>",
		Short: "Create an HTTP source",
		Long:  `Create a source pointing at a tar/zip archive served over HTTP(S).`,
		Example: `  # Public archive
  admiral source create http acme-release \
    --url https://artifacts.example.com/releases/v1.2.3.tar.gz

  # Protected archive with bearer auth
  admiral source create http acme-release \
    --url https://artifacts.acme.internal/releases/v1.2.3.tar.gz \
    --credential acme-artifacts-token`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &sourcev1.CreateSourceRequest{
				Name: args[0],
				Type: sourcev1.SourceType_SOURCE_TYPE_HTTP,
			}
			return finalizeCreate(cmd, opts, req, &flags)
		},
	}
	flags.bind(cmd)
	return cmd
}