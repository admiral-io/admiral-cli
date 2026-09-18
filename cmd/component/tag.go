package component

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newTagCmd(opts *client.Options) *cobra.Command {
	var digest string

	cmd := &cobra.Command{
		Use:   "tag <name>:<tag> --digest <digest>",
		Short: "Point a tag at a revision",
		Long: `Point a tag at a revision.

A semver tag (v1.2.0) is set once and never moves; pointing it elsewhere is
refused. A floating tag (latest, main) may be moved freely. The digest is
the revision's full sha256, as printed by publish or 'revision list -o wide'.`,
		Example: `  # Release a revision under a version
  admiral component tag cloud-sql:v1.2.0 --digest sha256:3f9a...

  # Move a floating tag
  admiral component tag cloud-sql:latest --digest sha256:3f9a...`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tag, err := splitRef(args[0])
			if err != nil {
				return err
			}
			if tag == "" {
				return cmderr.Usage("%q names no tag; use <name>:<tag>", args[0])
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			componentID, err := resolve.Component(cmd.Context(), c.Registry(), name)
			if err != nil {
				return err
			}
			resp, err := c.Registry().SetTag(cmd.Context(), &registryv1.SetTagRequest{
				ComponentId: componentID, Name: tag, Digest: digest,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Writef(p.Err(), "Tagged %s:%s -> %s\n", name, resp.Tag.Name, shortDigest(resp.Tag.Digest))
			return p.PrintOne(resp.Tag, name+":"+resp.Tag.Name, tagTable.Render(p, resp.Tag))
		},
	}

	cmd.Flags().StringVar(&digest, "digest", "", "the revision to name, as sha256:<hex>")
	_ = cmd.MarkFlagRequired("digest")

	return cmd
}
