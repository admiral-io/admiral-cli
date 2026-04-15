package source

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		srcID          string
		newName        string
		description    string
		newURL         string
		credentialName string
		credentialID   string
		clearCred      bool
		catalog        bool
		labelStrs      []string
	)

	cmd := &cobra.Command{
		Use:   "update [source]",
		Short: "Update a source",
		Long: `Update an existing source.

Updateable: name, description, url, credential, catalog flag, labels.
The source's TYPE is immutable -- to switch type, delete and recreate.`,
		Example: `  # Repoint the URL
  admiral source update acme-infra --url https://github.com/acme/infra-v2.git

  # Detach the credential (e.g. repo became public)
  admiral source update acme-infra --clear-credential

  # Swap to a different credential
  admiral source update acme-infra --credential acme-github-pat-v2

  # Mark as catalog
  admiral source update corp-rds --catalog`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameArg string
			if len(args) == 1 {
				nameArg = args[0]
			}
			if nameArg == "" && srcID == "" {
				_ = cmd.Help()
				return fmt.Errorf("source name or --id is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := util.ResolveSourceID(cmd.Context(), c.Source(), nameArg, srcID)
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
			if cmd.Flags().Changed("catalog") {
				s.Catalog = catalog
				paths = append(paths, "catalog")
			}
			if cmd.Flags().Changed("label") {
				labels, err := util.ParseLabels(labelStrs)
				if err != nil {
					return err
				}
				s.Labels = labels
				paths = append(paths, "labels")
			}

			credChanged := cmd.Flags().Changed("credential") || cmd.Flags().Changed("credential-id") || cmd.Flags().Changed("clear-credential")
			if credChanged {
				if clearCred && (cmd.Flags().Changed("credential") || cmd.Flags().Changed("credential-id")) {
					return fmt.Errorf("--clear-credential is mutually exclusive with --credential / --credential-id")
				}
				if clearCred {
					s.CredentialId = nil
				} else {
					var newCredID string
					if credentialID != "" {
						newCredID = credentialID
					} else {
						resolved, err := util.ResolveCredentialID(cmd.Context(), c.Credential(), credentialName, "")
						if err != nil {
							return err
						}
						newCredID = resolved
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

			p := output.NewPrinter(opts.OutputFormat)
			return p.PrintResource(resp, func(w *tabwriter.Writer) {
				printSourceRow(w, resp.Source)
			})
		},
	}

	cmd.Flags().StringVar(&srcID, "id", "", "source ID (UUID)")
	cmd.Flags().StringVar(&newName, "name", "", "new source name")
	cmd.Flags().StringVar(&description, "description", "", "source description")
	cmd.Flags().StringVar(&newURL, "url", "", "new source URL")
	cmd.Flags().StringVar(&credentialName, "credential", "", "attach credential by name")
	cmd.Flags().StringVar(&credentialID, "credential-id", "", "attach credential by UUID")
	cmd.Flags().BoolVar(&clearCred, "clear-credential", false, "detach the credential (make source anonymous)")
	cmd.Flags().BoolVar(&catalog, "catalog", false, "catalog flag")
	util.AddLabelFlag(cmd, &labelStrs, "label to set (key=value, repeatable)")
	return cmd
}