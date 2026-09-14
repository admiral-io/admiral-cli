package source

import (
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		newName        string
		description    string
		newURL         string
		credentialName string
		clearCred      bool
		labelStrs      []string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a source",
		Long: `Update an existing source.

Updateable: name, description, url, credential, labels.
The source's TYPE is immutable -- to switch type, delete and recreate.`,
		Example: `  # Repoint the URL
  admiral source update acme-infra --url https://github.com/acme/infra-v2.git

  # Detach the credential (e.g. repo became public)
  admiral source update acme-infra --clear-credential

  # Swap to a different credential
  admiral source update acme-infra --credential acme-github-pat-v2`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Source(cmd.Context(), c.Source(), args[0])
			if err != nil {
				return err
			}

			current, err := c.Source().GetSource(cmd.Context(), &sourcev1.GetSourceRequest{SourceId: id})
			if err != nil {
				return err
			}
			s := current.Source

			var paths []string
			if cmd.Flags().Changed("name") {
				s.Name = newName
				paths = append(paths, "name")
			}
			if cmd.Flags().Changed("description") {
				s.Description = description
				paths = append(paths, "description")
			}
			if cmd.Flags().Changed("url") {
				s.Url = newURL
				paths = append(paths, "url")
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(s.Labels, labelStrs)
				if err != nil {
					return err
				}
				s.Labels = labels
				paths = append(paths, "labels")
			}

			credChanged := cmd.Flags().Changed("credential") || cmd.Flags().Changed("clear-credential")
			if credChanged {
				if clearCred && cmd.Flags().Changed("credential") {
					return cmderr.Usage("--clear-credential and --credential are mutually exclusive")
				}
				if clearCred {
					s.CredentialId = nil
				} else {
					newCredID, err := resolve.Credential(cmd.Context(), c.Credential(), credentialName)
					if err != nil {
						return err
					}
					s.CredentialId = &newCredID
				}
				paths = append(paths, "credential_id")
			}

			if len(paths) == 0 {
				return fmt.Errorf("at least one updateable field must be specified")
			}

			resp, err := c.Source().UpdateSource(cmd.Context(), &sourcev1.UpdateSourceRequest{
				Source:     s,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Source, resp.Source.Name, sourceTable.Render(p, resp.Source))
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new source name")
	cmd.Flags().StringVar(&description, "description", "", "source description")
	cmd.Flags().StringVar(&newURL, "url", "", "new source URL")
	cmd.Flags().StringVar(&credentialName, "credential", "", "credential name or ID to attach")
	cmd.Flags().BoolVar(&clearCred, "clear-credential", false, "detach the credential (make source anonymous)")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	return cmd
}
