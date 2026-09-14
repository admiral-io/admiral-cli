package catalog

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	cliflags "go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	catalogv1 "go.admiral.io/sdk/proto/admiral/api/catalog/v1"
)

var catalogItemTypeFromString = map[string]catalogv1.CatalogItemType{
	"terraform": catalogv1.CatalogItemType_CATALOG_ITEM_TYPE_TERRAFORM,
	"helm":      catalogv1.CatalogItemType_CATALOG_ITEM_TYPE_HELM,
	"kustomize": catalogv1.CatalogItemType_CATALOG_ITEM_TYPE_KUSTOMIZE,
	"manifest":  catalogv1.CatalogItemType_CATALOG_ITEM_TYPE_MANIFEST,
}

type createFlags struct {
	description string
	sourceName  string
	ref         string
	root        string
	path        string
	labelStrs   []string
}

func (f *createFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.description, "description", "", "catalog item description")
	cmd.Flags().StringVar(&f.sourceName, "source", "", "source name or ID (required)")
	cmd.Flags().StringVar(&f.ref, "ref", "", "git ref / registry version / chart version")
	cmd.Flags().StringVar(&f.root, "root", "", "subdirectory within the fetched tree")
	cmd.Flags().StringVar(&f.path, "path", "", "working directory within root for execution")
	cliflags.Label(cmd, &f.labelStrs, "label to attach (key=value, repeatable)")
}

func newCreateCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a catalog item",
		Long: `Create a catalog item by selecting a type subcommand.

Subcommands:
  terraform    Terraform root module
  helm         Helm chart
  kustomize    Kustomize overlay
  manifest     Raw Kubernetes manifests`,
		Args: cliflags.NoArgs,
	}

	cmd.AddCommand(
		newCreateTypeCmd(opts, "terraform", "Create a TERRAFORM catalog item",
			`Create a catalog item pointing at a Terraform root module.

The source must be a GIT, TERRAFORM, or HTTP source.`,
			`  admiral catalog create terraform billing-vpc \
    --source acme-infra-repo --ref v1.2.3 --root modules/vpc`),
		newCreateTypeCmd(opts, "helm", "Create a HELM catalog item",
			`Create a catalog item pointing at a Helm chart.

The source must be a GIT, HELM, OCI, or HTTP source.`,
			`  admiral catalog create helm nginx \
    --source bitnami-charts --ref 15.1.0`),
		newCreateTypeCmd(opts, "kustomize", "Create a KUSTOMIZE catalog item",
			`Create a catalog item pointing at a Kustomize overlay.

The source must be a GIT or HTTP source.`,
			`  admiral catalog create kustomize api-overlay \
    --source platform-repo --root overlays/production`),
		newCreateTypeCmd(opts, "manifest", "Create a MANIFEST catalog item",
			`Create a catalog item pointing at raw Kubernetes manifests.

The source must be a GIT or HTTP source.`,
			`  admiral catalog create manifest monitoring \
    --source platform-repo --root k8s/monitoring`),
	)

	return cmd
}

func newCreateTypeCmd(opts *client.Options, typeName, short, long, example string) *cobra.Command {
	var flags createFlags

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s <name>", typeName),
		Short:   short,
		Long:    long,
		Example: example,
		Args:    cliflags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			modType, ok := catalogItemTypeFromString[strings.ToLower(typeName)]
			if !ok {
				return fmt.Errorf("unknown catalog item type: %s", typeName)
			}

			if flags.sourceName == "" {
				return cmderr.Usage("--source is required")
			}

			labels, err := cliflags.ParseLabels(flags.labelStrs)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			sourceID, err := resolve.Source(cmd.Context(), c.Source(), flags.sourceName)
			if err != nil {
				return err
			}

			resp, err := c.Catalog().CreateCatalogItem(cmd.Context(), &catalogv1.CreateCatalogItemRequest{
				Name:        args[0],
				Description: flags.description,
				Type:        modType,
				SourceId:    sourceID,
				Ref:         flags.ref,
				Root:        flags.root,
				Path:        flags.path,
				Labels:      labels,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.CatalogItem, resp.CatalogItem.Name, catalogItemTable.Render(p, resp.CatalogItem))
		},
	}

	flags.bind(cmd)
	return cmd
}
