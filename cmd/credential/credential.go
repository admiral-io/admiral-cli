package credential

import (
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.admiral.io/cli/internal/output"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"

	"go.admiral.io/cli/internal/client"
)

// CredentialCmd is the parent command for credential operations.
type CredentialCmd struct {
	Cmd *cobra.Command
}

// NewCredentialCmd creates the credential command tree.
func NewCredentialCmd(opts *client.Options) *CredentialCmd {
	root := &CredentialCmd{}

	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage credentials",
		Long: `Manage credentials used to authenticate to external systems.

Credentials are mechanism-rooted: the type describes the auth shape
(SSH key, basic auth, bearer token), not the target system. A single
credential may be reused across many Sources of compatible types.`,
		Aliases:       []string{"cred", "credentials"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
	)

	root.Cmd = cmd
	return root
}

var credentialTypeLabel = map[credentialv1.CredentialType]string{
	credentialv1.CredentialType_CREDENTIAL_TYPE_SSH_KEY:      "SSH_KEY",
	credentialv1.CredentialType_CREDENTIAL_TYPE_BASIC_AUTH:   "BASIC_AUTH",
	credentialv1.CredentialType_CREDENTIAL_TYPE_BEARER_TOKEN: "BEARER_TOKEN",
}

func formatCredentialType(t credentialv1.CredentialType) string {
	if s, ok := credentialTypeLabel[t]; ok {
		return s
	}
	return t.String()
}

// printCredentialRow writes a single-row summary (header + values) to w.
func printCredentialRow(w *tabwriter.Writer, c *credentialv1.Credential) {
	output.Writeln(w, "NAME\tTYPE\tLABELS\tAGE")
	output.Writef(w, "%s\t%s\t%s\t%s\n",
		c.Name,
		formatCredentialType(c.Type),
		output.FormatLabels(c.Labels),
		output.FormatAge(c.CreatedAt),
	)
}
