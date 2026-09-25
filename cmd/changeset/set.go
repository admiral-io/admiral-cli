package changeset

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

func newSetCmd(opts *client.Options) *cobra.Command {
	var (
		to              string
		setStrings      []string
		namespace       string
		extraNamespaces []string
		ifRev           int32
		po              planOptions
	)

	cmd := &cobra.Command{
		Use:   "set <change-set> <component>.<path>=<value>...",
		Short: "Set values, move a component's pin, or place it",
		Long: `Set values, move a component's pin, or place it. One command cuts one
revision.

A value reads as YAML, as with helm --set: 3 is a number, true a bool,
"3" a string, null clears a chart default, !ref users-db.host references
another component's output. --set-string keeps the characters as a string.
A dot inside a key is written \. or the key is quoted: api.annotations."a.b"=x.

With --to, the one argument after the change set is a component, and its
pin moves to that tag or digest of the same registry component.

With --namespace, the argument after the change set is a component, and its
placement is replaced: the namespace its objects go to, and with
--extra-namespace the others it may also write to. Values given after it
land in the same revision.`,
		Example: `  admiral changeset set cs-7f2a1c9d0e3b api.image.tag=v1.4.0 api.replicas=3

  # A value that must stay a string
  admiral changeset set cs-7f2a1c9d0e3b --set-string api.build=0042

  # Refuse if someone else edited it since revision 4
  admiral changeset set cs-7f2a1c9d0e3b api.replicas=3 --if-revision 4

  # Move the pin
  admiral changeset set cs-7f2a1c9d0e3b api --to v1.4.0

  # Put a component in its own namespace, allowed to write to kube-system
  admiral changeset set cs-7f2a1c9d0e3b api --namespace shop-api --extra-namespace kube-system

  # Back to the environment's namespace
  admiral changeset set cs-7f2a1c9d0e3b api --namespace ""`,
		Args: flags.RangeArgs(1, 1+maxEdits),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}

			es, err := setRequest{
				args:            args[1:],
				to:              to,
				setStrings:      setStrings,
				namespaceGiven:  cmd.Flags().Changed("namespace"),
				namespace:       namespace,
				extraNamespaces: extraNamespaces,
			}.edits()
			if err != nil {
				return err
			}
			return edits(cmd, opts, csID, ifRevision(cmd, ifRev), po, es...)
		},
	}

	cmd.Flags().StringVar(&to, "to", "", "move the component's pin to this tag or sha256: digest")
	cmd.Flags().StringVar(&namespace, "namespace", "", `put the component in this Kubernetes namespace; "" means the environment's default`)
	cmd.Flags().StringArrayVar(&extraNamespaces, "extra-namespace", nil, "a namespace the component may also write to, such as kube-system (repeatable)")
	cmd.Flags().StringArrayVar(&setStrings, "set-string", nil, "set <component>.<path>=<value> as a string, whatever it looks like (repeatable)")
	revisionFlag(cmd, &ifRev)
	planFlags(cmd, &po)

	return cmd
}

// setRequest is what one set command was given, after the change set.
type setRequest struct {
	args            []string
	to              string
	setStrings      []string
	namespaceGiven  bool
	namespace       string
	extraNamespaces []string
}

// edits builds the one request a set sends: a placement first when
// --namespace names a component, then the pin move or the values.
func (r setRequest) edits() ([]*changesetv1.Edit, error) {
	var es []*changesetv1.Edit
	assignments := r.args
	if r.namespaceGiven || len(r.extraNamespaces) > 0 {
		if !r.namespaceGiven {
			return nil, cmderr.UsageHint(`Pass --namespace as well; --namespace "" means the environment's default.`,
				"--extra-namespace needs --namespace, since the placement is replaced whole")
		}
		if len(r.args) == 0 || strings.ContainsAny(r.args[0], ".=") {
			return nil, cmderr.UsageHint("Name the component first: 'admiral changeset set cs-… api --namespace shop'.",
				"--namespace takes a component")
		}
		place, err := placementEdit(r.args[0], r.namespace, r.extraNamespaces)
		if err != nil {
			return nil, err
		}
		es = append(es, place)
		assignments = r.args[1:]
	}

	if r.to != "" {
		if len(r.setStrings) > 0 {
			return nil, cmderr.Usage("--to cannot be combined with --set-string")
		}
		if len(r.args) != 1 || strings.ContainsAny(r.args[0], ".=") {
			return nil, cmderr.UsageHint("Move one pin per command: 'admiral changeset set cs-… api --to v1.4.0'.",
				"--to takes exactly one component")
		}
		if err := componentName(r.args[0]); err != nil {
			return nil, err
		}
		return append(es, &changesetv1.Edit{
			Edit: &changesetv1.Edit_SetPin{SetPin: &changesetv1.SetPin{Component: r.args[0], Reference: r.to}},
		}), nil
	}

	values, err := setEdits(assignments, r.setStrings)
	if err != nil {
		return nil, err
	}
	es = append(es, values...)
	if len(es) == 0 {
		return nil, cmderr.UsageHint("Give <component>.<path>=<value>, --set-string, or a component with --to or --namespace.",
			"nothing to set")
	}
	return es, nil
}
