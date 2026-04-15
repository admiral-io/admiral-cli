package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newCreateOCICmd(opts *client.Options) *cobra.Command {
	var flags commonCreateFlags

	cmd := &cobra.Command{
		Use:   "oci <name>",
		Short: "Create an OCI source",
		Long:  `Create a source pointing at an OCI Distribution Spec registry repository.`,
		Example: `  # GHCR with basic-auth credential
  admiral source create oci acme-charts \
    --url oci://ghcr.io/acme/charts --credential acme-github

  # Registry with bearer-token credential
  admiral source create oci acme-internal \
    --url oci://registry.acme.internal/platform \
    --credential acme-registry-token`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &sourcev1.CreateSourceRequest{
				Name: args[0],
				Type: sourcev1.SourceType_SOURCE_TYPE_OCI,
			}
			return finalizeCreate(cmd, opts, req, &flags)
		},
	}
	flags.bind(cmd)
	return cmd
}