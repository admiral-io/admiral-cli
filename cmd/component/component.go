package component

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
)

type ComponentCmd struct {
	Cmd *cobra.Command
}

// NewComponentCmd is the registry: what has been published, addressed by
// name and tag. Publish, tag and deprecate act here; add, update and copy
// act inside a change set, on the other thing called a component.
func NewComponentCmd(opts *client.Options) *ComponentCmd {
	root := &ComponentCmd{}

	cmd := &cobra.Command{
		Use:   "component",
		Short: "Publish and inspect components in the registry",
		Long: `Publish and inspect components in the registry.

A component here is a registry entry: a Terraform module, a Helm chart or a
set of manifests, published as an immutable revision identified by the
digest of its bytes. Tags name revisions the way image tags name digests;
a semver tag (v1.2.0) never moves once set, a floating one (latest) may.

References: NAME:TAG (cloud-sql:v1.2.0) or NAME@DIGEST (cloud-sql@sha256:...).`,
		Aliases:       []string{"components"},
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}

	cmd.AddCommand(
		newPublishCmd(opts),
		newListCmd(opts),
		newGetCmd(opts),
	)

	root.Cmd = cmd
	return root
}
