package source

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/util"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
)

func newCreateGitCmd(opts *client.Options) *cobra.Command {
	var flags commonCreateFlags

	cmd := &cobra.Command{
		Use:   "git <name>",
		Short: "Create a GIT source",
		Long:  `Create a source pointing at a Git repository (HTTPS or SSH).`,
		Example: `  # HTTPS with a BASIC_AUTH credential
  admiral source create git acme-infra \
    --url https://github.com/acme/infrastructure.git \
    --credential acme-github

  # SSH with an SSH_KEY credential
  admiral source create git acme-infra \
    --url git@github.com:acme/infrastructure.git \
    --credential acme-deploy`,
		Args: util.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &sourcev1.CreateSourceRequest{
				Name: args[0],
				Type: sourcev1.SourceType_SOURCE_TYPE_GIT,
			}
			return finalizeCreate(cmd, opts, req, &flags)
		},
	}
	flags.bind(cmd)
	return cmd
}