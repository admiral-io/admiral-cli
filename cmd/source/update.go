package source

import (
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		description string
		allows      []string
		credential  string
		integration string
		clearGrant  bool
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a source's description, allows or grant",
		Long: `Update a source's description, what it allows, or the credential or
integration that reaches it. Only what you pass changes. The kind and the
location never change: a moved upstream is a new source.

Tightening --allows never touches what already runs; it stops new imports.`,
		Example: `  # Let every chart of the repository through
  admiral source update argo-helm --allows '*'

  # Replace the credential
  admiral source update infra --credential infra-deploy-2

  # The upstream went public
  admiral source update charts --clear-grant`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Sources(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := updatePaths(cmd, credential, integration, clearGrant)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			src, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("description") {
				src.Description = description
			}

			if cmd.Flags().Changed("allows") {
				src.Allows = allows
			}

			if err := applyGrant(cmd, c.Credential(), src, credential, integration, clearGrant); err != nil {
				return err
			}

			resp, err := c.Source().UpdateSource(cmd.Context(), &sourcev1.UpdateSourceRequest{
				Source:     src,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)

			return p.PrintOne(resp.Source, resp.Source.Name, sourceTable.Render(p, resp.Source))
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringArrayVar(&allows, "allows", nil, "replace what may come through with these (repeatable)")
	cmd.Flags().StringVar(&credential, "credential", "", "the credential that reaches the upstream, by name or ID")
	complete.Flag(cmd, "credential", complete.Credentials(opts))
	cmd.Flags().StringVar(&integration, "integration", "", "the GitHub integration that reaches the repository, by ID")
	cmd.Flags().BoolVar(&clearGrant, "clear-grant", false, "fetch the upstream with no credential")

	return cmd
}

// updatePaths is the mask an update sends, refused locally when it names
// nothing or more than one way to set the grant.
func updatePaths(cmd *cobra.Command, credential, integration string, clearGrant bool) ([]string, error) {
	var paths []string
	if cmd.Flags().Changed("description") {
		paths = append(paths, "description")
	}

	if cmd.Flags().Changed("allows") {
		paths = append(paths, "allows")
	}

	grants := 0

	for _, set := range []bool{credential != "", integration != "", clearGrant} {
		if set {
			grants++
		}
	}

	if grants > 1 {
		return nil, cmderr.Usage("pass one of --credential, --integration or --clear-grant")
	}

	if grants == 1 {
		paths = append(paths, "credential_id", "integration_id")
	}

	if len(paths) == 0 {
		return nil, cmderr.Usage("at least one of --description, --allows, --credential, --integration or --clear-grant must be specified")
	}

	return paths, nil
}

// applyGrant sets the grant the flags name on the source, resolving a
// credential's name to its ID.
func applyGrant(cmd *cobra.Command, creds credentialv1.CredentialAPIClient, src *sourcev1.Source, credential, integration string, clearGrant bool) error {
	switch {
	case credential != "":
		id, err := resolve.Credential(cmd.Context(), creds, credential)
		if err != nil {
			return err
		}

		src.Grant = &sourcev1.Source_CredentialId{CredentialId: id}
	case integration != "":
		src.Grant = &sourcev1.Source_IntegrationId{IntegrationId: integration}
	case clearGrant:
		src.Grant = nil
	}

	return nil
}
