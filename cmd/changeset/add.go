package changeset

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newAddCmd(opts *client.Options) *cobra.Command {
	var (
		from       string
		valuesPath string
		ifRev      int32
	)

	cmd := &cobra.Command{
		Use:   "add <change-set> <name> --from <component>:<tag>",
		Short: "Add a component from the registry",
		Long: `Add a component from the registry, under a name unique in the environment.

The name is reserved until the change set is discarded or the component is
removed from it. A tag is resolved to its revision now; only the revision is
stored.`,
		Example: `  admiral changeset add cs-7f2a1c9d0e3b users-db --from cloud-sql:v1.2.0

  # With values, pinned by digest
  admiral changeset add cs-7f2a1c9d0e3b users-db --from cloud-sql@sha256:3f9a... --values users-db.yaml`,
		Args: flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			if err := componentName(args[1]); err != nil {
				return err
			}
			src, err := registryRef(from)
			if err != nil {
				return err
			}
			add := &changesetv1.AddComponent{Name: args[1], Source: src}
			if valuesPath != "" {
				tree, err := readValues(cmd, valuesPath)
				if err != nil {
					return err
				}
				if add.ValuesJson, err = valuesfile.EncodeJSON(tree); err != nil {
					return err
				}
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev),
				&changesetv1.Edit{Edit: &changesetv1.Edit_AddComponent{AddComponent: add}})
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "the registry component, as <component>:<tag> or <component>@<digest>")
	cmd.Flags().StringVar(&valuesPath, "values", "", "a YAML values file; !ref <component>.<output> references another component")
	revisionFlag(cmd, &ifRev)
	_ = cmd.MarkFlagRequired("from")
	complete.Flag(cmd, "from", complete.Refs(opts))

	return cmd
}

// registryRef reads NAME:TAG or NAME@DIGEST, the form the component
// commands use. The namespace stays empty, which is the default one.
func registryRef(s string) (*changesetv1.RegistryRef, error) {
	var name, ref string
	var found bool
	if i := strings.Index(s, "@"); i >= 0 {
		name, ref, found = s[:i], s[i+1:], true
	} else {
		name, ref, found = strings.Cut(s, ":")
	}
	if !found || ref == "" {
		return nil, cmderr.UsageHint("Pin a revision: --from cloud-sql:v1.2.0 or --from cloud-sql@sha256:<hex>.",
			"--from %q names no tag or digest", s)
	}
	if err := componentName(name); err != nil {
		return nil, err
	}
	return &changesetv1.RegistryRef{Name: name, Reference: ref}, nil
}

// readValues reads and parses a values file. The warnings name each map
// that was stored escaped, which is what a person meant as !ref more often
// than not.
func readValues(cmd *cobra.Command, path string) (map[string]any, error) {
	data, err := readValuesFile(path)
	if err != nil {
		return nil, err
	}
	tree, warnings, err := valuesfile.Parse(data)
	if err != nil {
		return nil, cmderr.Usage("%s: %v", path, err)
	}
	for _, w := range warnings {
		output.Writef(cmd.ErrOrStderr(), "Warning: %s: %s\n", path, w)
	}
	return tree, nil
}

func readValuesFile(path string) ([]byte, error) {
	p, err := input.ExpandPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read values: %w", err)
	}
	return data, nil
}
