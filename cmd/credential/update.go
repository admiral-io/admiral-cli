package credential

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
)

func newUpdateCmd(opts *client.Options) *cobra.Command {
	var (
		description       string
		labelStrs         []string
		allowedHosts      []string
		clearAllowedHosts bool
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a credential's description, labels or allowed hosts",
		Long: `Update a credential's description, labels or allowed hosts. Only the
fields you pass are changed. The secret is replaced with 'credential
rotate'; the type cannot change.`,
		Example: `  # Narrow where it may be presented
  admiral credential update github --allowed-host github.com --allowed-host ghcr.io

  # Lift the guard
  admiral credential update github --clear-allowed-hosts

  # Set one label and remove another
  admiral credential update github --label owner=platform --label temp-`,
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Credentials(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			var paths []string
			if cmd.Flags().Changed("description") {
				paths = append(paths, "description")
			}
			if cmd.Flags().Changed("label") {
				paths = append(paths, "labels")
			}
			if cmd.Flags().Changed("allowed-host") || clearAllowedHosts {
				if cmd.Flags().Changed("allowed-host") && clearAllowedHosts {
					return cmderr.Usage("pass either --allowed-host or --clear-allowed-hosts, not both")
				}
				paths = append(paths, "allowed_hosts")
			}
			if len(paths) == 0 {
				return cmderr.Usage("at least one of --description, --label, --allowed-host or --clear-allowed-hosts must be specified")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			id, err := resolve.Credential(cmd.Context(), c.Credential(), args[0])
			if err != nil {
				return err
			}
			// Read-modify-write so untouched fields keep their values.
			current, err := c.Credential().GetCredential(cmd.Context(), &credentialv1.GetCredentialRequest{CredentialId: id})
			if err != nil {
				return err
			}
			cred := current.Credential

			if cmd.Flags().Changed("description") {
				cred.Description = description
			}
			if cmd.Flags().Changed("label") {
				labels, err := flags.ApplyLabelPatch(cred.Labels, labelStrs)
				if err != nil {
					return err
				}
				cred.Labels = labels
			}
			if cmd.Flags().Changed("allowed-host") {
				cred.AllowedHosts = allowedHosts
			}
			if clearAllowedHosts {
				cred.AllowedHosts = nil
			}

			resp, err := c.Credential().UpdateCredential(cmd.Context(), &credentialv1.UpdateCredentialRequest{
				Credential: cred,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Credential, resp.Credential.Name, credentialTable.Render(p, resp.Credential))
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "new description")
	flags.Label(cmd, &labelStrs, "patch labels: key=value to set, key- to remove (repeatable)")
	cmd.Flags().StringArrayVar(&allowedHosts, "allowed-host", nil, "replace the allowed hosts with these (repeatable)")
	cmd.Flags().BoolVar(&clearAllowedHosts, "clear-allowed-hosts", false, "allow any host the type fits")

	return cmd
}
