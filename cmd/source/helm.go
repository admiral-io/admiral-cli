package source

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newCreateHelmCmd(opts *client.Options) *cobra.Command {
	var (
		flags     commonCreateFlags
		chartName string
	)

	cmd := &cobra.Command{
		Use:   "helm <name>",
		Short: "Create a HELM source",
		Long:  `Create a source pointing at a chart in a Helm HTTP chart repository.`,
		Example: `  # Public chart repository
  admiral source create helm bitnami-nginx \
    --url https://charts.bitnami.com/bitnami --chart-name nginx

  # Private repository with bearer auth
  admiral source create helm acme-payments \
    --url https://charts.acme.internal --chart-name payments \
    --credential acme-helm-token`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if chartName == "" {
				return fmt.Errorf("--chart-name is required")
			}
			req := &sourcev1.CreateSourceRequest{
				Name: args[0],
				Type: sourcev1.SourceType_SOURCE_TYPE_HELM,
				SourceConfig: &sourcev1.CreateSourceRequest_Helm{
					Helm: &sourcev1.HelmConfig{ChartName: chartName},
				},
			}
			return finalizeCreate(cmd, opts, req, &flags)
		},
	}
	flags.bind(cmd)
	cmd.Flags().StringVar(&chartName, "chart-name", "", "chart name within the repository (required)")
	return cmd
}