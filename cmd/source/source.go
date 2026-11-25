package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type SourceCmd struct {
	Cmd *cobra.Command
}

// NewSourceCmd manages the organization's sources, the one place an
// upstream location is typed.
func NewSourceCmd(opts *client.Options) *SourceCmd {
	root := &SourceCmd{}

	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage the upstreams components are imported from",
		Long: `Manage the upstreams components are imported from.

A source is one upstream location: a Helm repository, an OCI registry
namespace, a Terraform registry namespace or a git repository. Its URL is
typed once, here, and everything after names the source: 'source search'
lists what it offers, and 'component import --source' brings one artifact
in as a component.

A source bounds what may come through it with --allows: chart, module or
path names, exact or ending in *, optionally with a version range after @,
such as argo-rollouts@>=2.39. Left off, it allows everything, and the
output says so.

A public upstream needs no credential. A private one takes --credential or
--integration, which needs permission to grant sources, held by
organization admins. Creating a source needs permission to create sources,
also an admin's unless granted further.

Commands accept a source name or ID as the positional argument.`,
		Aliases:       []string{"sources", "src"},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)

	cmd.AddCommand(
		newCreateCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
		newUpdateCmd(opts),
		newDeleteCmd(opts),
		newSearchCmd(opts),
	)

	root.Cmd = cmd

	return root
}
