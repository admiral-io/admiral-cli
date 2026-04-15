package source

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sdkclient "go.admiral.io/sdk/client"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

// commonCreateFlags groups the flags shared by every `source create <type>` subcommand.
type commonCreateFlags struct {
	description    string
	url            string
	credentialName string
	credentialID   string
	catalog        bool
	labelStrs      []string
}

func (f *commonCreateFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.description, "description", "", "source description")
	cmd.Flags().StringVar(&f.url, "url", "", "source URL (required)")
	cmd.Flags().StringVar(&f.credentialName, "credential", "", "credential name to attach (omit for public sources)")
	cmd.Flags().StringVar(&f.credentialID, "credential-id", "", "credential UUID to attach (alternative to --credential)")
	cmd.Flags().BoolVar(&f.catalog, "catalog", false, "mark as a curated catalog source")
	util.AddLabelFlag(cmd, &f.labelStrs, "label to attach (key=value, repeatable)")
}

// resolveCredential returns the credential UUID to attach, or empty for none.
func (f *commonCreateFlags) resolveCredential(ctx context.Context, c sdkclient.AdmiralClient) (*string, error) {
	if f.credentialID != "" {
		id := f.credentialID
		return &id, nil
	}
	if f.credentialName == "" {
		return nil, nil
	}
	id, err := util.ResolveCredentialID(ctx, c.Credential(), f.credentialName, "")
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
		Args: cobra.NoArgs,
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
		return fmt.Errorf("--url is required")
	}
	req.Url = flags.url
	req.Description = flags.description
	req.Catalog = flags.catalog

	labels, err := util.ParseLabels(flags.labelStrs)
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

	p := output.NewPrinter(opts.OutputFormat)
	return p.PrintResource(resp, func(w *tabwriter.Writer) {
		printSourceRow(w, resp.Source)
	})
}
