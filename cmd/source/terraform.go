package source

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	cliflags "go.admiral.io/cli/internal/flags"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newCreateTerraformCmd(opts *client.Options) *cobra.Command {
	var (
		flags      commonCreateFlags
		namespace  string
		moduleName string
		system     string
	)

	cmd := &cobra.Command{
		Use:   "terraform <name>",
		Short: "Create a TERRAFORM source",
		Long:  `Create a source pointing at a module in a Terraform Registry (HCP, TFE, private).`,
		Example: `  # HCP Terraform module
  admiral source create terraform acme-vpc \
    --url https://app.terraform.io \
    --tf-namespace acme --tf-module-name vpc --tf-system aws \
    --credential hcp-terraform

  # Private registry
  admiral source create terraform acme-network \
    --url https://tf.acme.internal \
    --tf-namespace platform --tf-module-name network --tf-system aws \
    --credential acme-tf-token`,
		Args: cliflags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" || moduleName == "" || system == "" {
				return fmt.Errorf("--tf-namespace, --tf-module-name, and --tf-system are all required")
			}
			req := &sourcev1.CreateSourceRequest{
				Name: args[0],
				Type: sourcev1.SourceType_SOURCE_TYPE_TERRAFORM,
				SourceConfig: &sourcev1.SourceConfig{
					Variant: &sourcev1.SourceConfig_Terraform{
						Terraform: &sourcev1.TerraformConfig{
							Namespace:  namespace,
							ModuleName: moduleName,
							System:     system,
						},
					},
				},
			}
			return finalizeCreate(cmd, opts, req, &flags)
		},
	}
	flags.bind(cmd)
	cmd.Flags().StringVar(&namespace, "tf-namespace", "", "registry namespace (e.g., 'hashicorp')")
	cmd.Flags().StringVar(&moduleName, "tf-module-name", "", "module name (e.g., 'consul')")
	cmd.Flags().StringVar(&system, "tf-system", "", "target system (e.g., 'aws', 'azurerm', 'google')")
	return cmd
}
