package source

import (
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newCreateCmd(opts *client.Options) *cobra.Command {
	var (
		locations   = map[string]*string{}
		allows      []string
		credential  string
		integration string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create <name> (--helm | --oci | --terraform | --git) LOCATION",
		Short: "Create a source",
		Long: `Create a source: name an upstream location once.

Exactly one of --helm, --oci, --terraform or --git says what the location
is. --allows bounds what may come through it, repeatable; left off, it
allows everything. A public upstream needs no credential; a private one
takes --credential or --integration.

A source that publishes when a GitHub repository is pushed is set up in the
console, where the repository is picked from the installation's list.`,
		Example: `  # Argo's public Helm repository, Argo Rollouts only
  admiral source create argo-helm --helm https://argoproj.github.io/argo-helm --allows argo-rollouts

  # cert-manager's, everything it offers
  admiral source create jetstack --helm https://charts.jetstack.io

  # A Terraform registry namespace, version 5 and up of one module
  admiral source create tf-aws --terraform registry.terraform.io/terraform-aws-modules --allows 'vpc/aws@>=5'

  # A private git repository, with a deploy key the organization holds
  admiral source create infra --git git@github.com:acme/infra.git --allows 'modules/*' --credential infra-deploy`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, location, err := locationOf(cmd, locations)
			if err != nil {
				return err
			}

			if credential != "" && integration != "" {
				return cmderr.Usage("a source takes --credential or --integration, not both")
			}

			if len(allows) == 0 {
				allows = []string{"*"}
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src := &sourcev1.Source{
				Name: args[0], Description: description, Kind: kind, Location: location,
				Trigger: sourcev1.Source_TRIGGER_REQUEST, Allows: allows,
			}

			switch {
			case credential != "":
				id, err := resolve.Credential(cmd.Context(), c.Credential(), credential)
				if err != nil {
					return err
				}

				src.Grant = &sourcev1.Source_CredentialId{CredentialId: id}
			case integration != "":
				src.Grant = &sourcev1.Source_IntegrationId{IntegrationId: integration}
			}

			resp, err := c.Source().CreateSource(cmd.Context(), &sourcev1.CreateSourceRequest{Source: src})
			if err != nil {
				return createRefused(err, integration != "")
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)

			return p.PrintOne(resp.Source, resp.Source.Name, sourceTable.Render(p, resp.Source))
		},
	}

	for _, k := range kinds {
		v := ""
		locations[k.flag] = &v
		cmd.Flags().StringVar(&v, k.flag, "", k.what)
	}

	cmd.Flags().StringArrayVar(&allows, "allows", nil, "what may come through, exact or ending in *, optionally @RANGE (repeatable; default: everything)")
	cmd.Flags().StringVar(&credential, "credential", "", "the credential that reaches a private upstream, by name or ID")
	complete.Flag(cmd, "credential", complete.Credentials(opts))
	cmd.Flags().StringVar(&integration, "integration", "", "the GitHub integration that reaches a repository, by ID")
	cmd.Flags().StringVar(&description, "description", "", "source description")

	return cmd
}

// locationOf is the one location flag that was given, refused locally when
// none or several were.
func locationOf(cmd *cobra.Command, locations map[string]*string) (sourcev1.Source_Kind, string, error) {
	var (
		kind  sourcev1.Source_Kind
		where string
		given []string
	)

	for _, k := range kinds {
		if v := strings.TrimSpace(*locations[k.flag]); cmd.Flags().Changed(k.flag) {
			given = append(given, "--"+k.flag)
			kind, where = k.kind, v
		}
	}

	switch {
	case len(given) == 0:
		return 0, "", cmderr.Usage("name the location with one of --helm, --oci, --terraform or --git")
	case len(given) > 1:
		return 0, "", cmderr.Usage("a source is one location; pass one of %s", strings.Join(given, ", "))
	case where == "":
		return 0, "", cmderr.Usage("%s needs a location", given[0])
	}

	return kind, where, nil
}

// createRefused explains a refused create. The API answers a caller who may
// not create sources with NotFound, so as not to confirm what exists. An
// integration is passed by ID unchecked, so its NotFound is left alone.
func createRefused(err error, namedIntegration bool) error {
	if namedIntegration || status.Code(err) != codes.NotFound {
		return err
	}

	return cmderr.WithHint(err, "Creating a source needs permission in this organization. "+
		"An organization admin can create it, or grant you source creation.")
}
