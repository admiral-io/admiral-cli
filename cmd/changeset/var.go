package changeset

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	variablev1 "go.admiral.io/sdk/proto/admiral/api/variable/v1"
)

var varTypeFromString = map[string]variablev1.VariableType{
	"STRING":  variablev1.VariableType_VARIABLE_TYPE_STRING,
	"NUMBER":  variablev1.VariableType_VARIABLE_TYPE_NUMBER,
	"BOOLEAN": variablev1.VariableType_VARIABLE_TYPE_BOOLEAN,
	"COMPLEX": variablev1.VariableType_VARIABLE_TYPE_COMPLEX,
}

// newVarCmd is the parent for change-set variable entries (set, remove).
// `--changeset` is a persistent flag inherited by every subcommand.
func newVarCmd(opts *client.Options) *cobra.Command {
	var csID string

	cmd := &cobra.Command{
		Use:           "var",
		Short:         "Stage variable changes inside a change set",
		Long:          `Stage variable upserts or deletions against an OPEN change set. Applied to the env's variable set on successful deploy.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}
	cmd.PersistentFlags().StringVar(&csID, "changeset", "", "change set ID or display ID (required)")

	cmd.AddCommand(
		newVarSetCmd(opts, &csID),
		newVarRemoveCmd(opts, &csID),
	)
	return cmd
}

func newVarSetCmd(opts *client.Options, csID *string) *cobra.Command {
	var (
		typeStr   string
		sensitive bool
	)

	cmd := &cobra.Command{
		Use:   "set <KEY> <VALUE>",
		Short: "Stage a variable set",
		Long:  `Add a variable entry to an OPEN change set. The variable is written to the env's variables on successful deploy.`,
		Example: `  admiral changeset var set --changeset cs-... TF_VAR_region "us-west-2"
  admiral changeset var set --changeset cs-... --type NUMBER replicas 3`,
		Args: flags.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			varType, ok := varTypeFromString[strings.ToUpper(typeStr)]
			if !ok {
				return fmt.Errorf("invalid --type %q (expected STRING, NUMBER, BOOLEAN, COMPLEX)", typeStr)
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().SetVariable(cmd.Context(), &changesetv1.SetVariableRequest{
				ChangeSetId: *csID,
				Key:         args[0],
				Value:       args[1],
				Type:        varType,
				Sensitive:   sensitive,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, varEntryTable.Render(p, resp.VariableEntry))
		},
	}
	flags.Enum(cmd, &typeStr, "type", "string", "value type", "string", "number", "boolean", "complex")
	cmd.Flags().BoolVar(&sensitive, "sensitive", false, "mark the value sensitive (encrypted at rest, masked on read)")
	return cmd
}

func newVarRemoveCmd(opts *client.Options, csID *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <KEY>",
		Aliases: []string{"rm"},
		Short:   "Stage a variable deletion (tombstone)",
		Long:    `Write a tombstone variable entry. On successful deploy, the key is removed from the env's variables.`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().RemoveVariable(cmd.Context(), &changesetv1.RemoveVariableRequest{
				ChangeSetId: *csID,
				Key:         args[0],
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, varEntryTable.Render(p, resp.VariableEntry))
		},
	}
	return cmd
}
