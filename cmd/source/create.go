package source

import (
	"context"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	cliflags "go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

// commonCreateFlags groups the flags shared by every `source create <type>` subcommand.
type commonCreateFlags struct {
	description    string
	url            string
	credentialName string
	labelStrs      []string
}

func (f *commonCreateFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.description, "description", "", "source description")
	cmd.Flags().StringVar(&f.url, "url", "", "source URL (required)")
	cmd.Flags().StringVar(&f.credentialName, "credential", "", "credential name or ID to attach (omit for public sources)")
	cliflags.Label(cmd, &f.labelStrs, "label to attach (key=value, repeatable)")
}

// resolveCredential returns the credential UUID to attach, or empty for none.
func (f *commonCreateFlags) resolveCredential(ctx context.Context, c sdkclient.AdmiralClient) (*string, error) {
	if f.credentialName == "" {
		return nil, nil
	}
	id, err := resolve.Credential(ctx, c.Credential(), f.credentialName)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func newCreateCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a source",
		Long: `Create a source by selecting a type subcommand.

Subcommands:
  git          Git repository (HTTPS or SSH)
  terraform    Terraform Module Registry (HCP, private registries)
  helm         Helm HTTP chart repository
  oci          OCI Distribution Spec registry
  http         Bare HTTP(S) archive (tar/zip)`,
		Args: cliflags.NoArgs,
	}

	cmd.AddCommand(
		newCreateGitCmd(opts),
		newCreateTerraformCmd(opts),
		newCreateHelmCmd(opts),
		newCreateOCICmd(opts),
		newCreateHTTPCmd(opts),
	)
	return cmd
}

// finalizeCreate handles the common tail of a create subcommand:
// resolve credential, send CreateSource, render the result.
func finalizeCreate(cmd *cobra.Command, opts *client.Options, req *sourcev1.CreateSourceRequest, flags *commonCreateFlags) error {
	if flags.url == "" {
		return cmderr.Usage("--url is required")
	}
	req.Url = flags.url
	req.Description = flags.description

	labels, err := cliflags.ParseLabels(flags.labelStrs)
	if err != nil {
		return err
	}
	req.Labels = labels

	c, err := client.CreateClient(cmd.Context(), opts)
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck

	credID, err := flags.resolveCredential(cmd.Context(), c)
	if err != nil {
		return err
	}
	req.CredentialId = credID

	resp, err := c.Source().CreateSource(cmd.Context(), req)
	if err != nil {
		return err
	}

	p := output.NewPrinter(cmd, opts.OutputFormat)
	return p.PrintOne(resp.Source, resp.Source.Name, sourceTable.Render(p, resp.Source))
}
