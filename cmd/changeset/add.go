package changeset

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/manifest"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/valuesfile"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newAddCmd(opts *client.Options) *cobra.Command {
	var (
		name       string
		valuesPath string
		sets       []string
		setStrings []string
		ifRev      int32
		po         planOptions
	)

	cmd := &cobra.Command{
		Use:   "add <changeset> <component>:<tag>",
		Short: "Add a component from the registry",
		Long: `Add a component from the registry, under a name unique in the environment.
The name is the registry component's unless --name gives another, which a
second copy of the same component needs.

The name is reserved until the changeset is discarded or the component is
removed from it. A tag is resolved to its version now; only the version is
stored.

Values come from --values, a YAML file or - for stdin, then each --set and
--set-string on top, as helm install applies them. A --set path starts
inside the component: --set image.tag=v2, not api.image.tag=v2. The
component and its values are one revision.`,
		Example: `  admiral changeset add cs-7f2a1c9d0e3b cloud-sql:v1.2.0

  # A second copy, under its own name
  admiral changeset add cs-7f2a1c9d0e3b cloud-sql:v1.2.0 --name users-db

  # With values, pinned by digest
  admiral changeset add cs-7f2a1c9d0e3b cloud-sql@sha256:3f9a... --values users-db.yaml

  # A value or two, without a file
  admiral changeset add cs-7f2a1c9d0e3b hello:v0.1.0 --set message="hello from dev"

  # Values from stdin
  admiral changeset add cs-7f2a1c9d0e3b hello:v0.1.0 --values - <<'YAML'
  message: hello from dev
  YAML`,
		Args: flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			src, err := registryRef(args[1])
			if err != nil {
				return err
			}
			if name == "" {
				name = src.Name
			}
			if err := componentName(name); err != nil {
				return err
			}
			var tree map[string]any
			if valuesPath != "" {
				if tree, err = readValues(cmd, valuesPath); err != nil {
					return err
				}
			}
			es, err := addEdits(name, src, tree, sets, setStrings)
			if err != nil {
				return err
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev), po, es...)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "the component's name in the environment (default the registry component's)")
	cmd.Flags().StringVar(&valuesPath, "values", "", "a YAML values file, or - for stdin; !ref <component>.<output> references another component")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "set <path>=<value> inside the component, read as YAML (repeatable)")
	cmd.Flags().StringArrayVar(&setStrings, "set-string", nil, "set <path>=<value> as a string, whatever it looks like (repeatable)")
	revisionFlag(cmd, &ifRev)
	planFlags(cmd, &po)
	cmd.ValidArgsFunction = complete.Second(complete.Refs(opts))

	return cmd
}

// addEdits is the one request an add sends: the component with its file
// values, then each --set on it, so the server applies a set exactly as
// 'changeset set' would and the whole add is one revision.
func addEdits(name string, src *changesetv1.RegistryRef, tree map[string]any, sets, setStrings []string) ([]*changesetv1.Edit, error) {
	add := &changesetv1.AddComponent{Name: name, Version: src}
	if tree != nil {
		js, err := valuesfile.EncodeJSON(tree)
		if err != nil {
			return nil, err
		}
		add.ValuesJson = js
	}
	es := []*changesetv1.Edit{{Edit: &changesetv1.Edit_AddComponent{AddComponent: add}}}
	values, err := setEdits(prefixed(name, sets), prefixed(name, setStrings))
	if err != nil {
		return nil, err
	}
	return append(es, values...), nil
}

// prefixed turns add's component-relative paths into set's
// component.path form. A path written with the component's name in front
// is not stripped: it is a key of that name inside the component.
func prefixed(name string, assignments []string) []string {
	out := make([]string, len(assignments))
	for i, a := range assignments {
		out[i] = name + "." + a
	}
	return out
}

// registryRef reads NAME:TAG or NAME@DIGEST, the form the component
// commands use.
func registryRef(s string) (*changesetv1.RegistryRef, error) {
	var name, ref string
	var found bool
	if i := strings.Index(s, "@"); i >= 0 {
		name, ref, found = s[:i], s[i+1:], true
	} else {
		name, ref, found = strings.Cut(s, ":")
	}
	if !found || ref == "" {
		return nil, cmderr.UsageHint("Name a version: cloud-sql:v1.2.0 or cloud-sql@sha256:<hex>.",
			"%q names no tag or digest", s)
	}
	if !manifest.ValidName(name) {
		return nil, cmderr.Usage("invalid registry component name %q: lowercase letters, digits and hyphens, starting with a letter", name)
	}
	return &changesetv1.RegistryRef{Name: name, Reference: ref}, nil
}

// readValues reads and parses a values file. The warnings name each map
// that was stored escaped, which is what a person meant as !ref more often
// than not.
func readValues(cmd *cobra.Command, path string) (map[string]any, error) {
	data, err := readValuesFile(cmd, path)
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

// readValuesFile reads path, or stdin when path is -.
func readValuesFile(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read values from stdin: %w", err)
		}
		return data, nil
	}
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
