package changeset

import (
	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
)

// newEntryCmd is the parent for per-component change-set entries
// (create, update, destroy, orphan, remove). The `--changeset` flag is
// declared as a persistent flag here so every subcommand inherits it.
func newEntryCmd(opts *client.Options) *cobra.Command {
	var csID string

	cmd := &cobra.Command{
		Use:           "entry",
		Short:         "Stage component changes inside a change set",
		Long:          `Stage CREATE / UPDATE / DESTROY / ORPHAN entries against components, or remove an entry from an OPEN change set.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          flags.NoArgs,
	}
	cmd.PersistentFlags().StringVar(&csID, "changeset", "", "change set ID or display ID (required)")

	cmd.AddCommand(
		newEntryCreateCmd(opts, &csID),
		newEntryUpdateCmd(opts, &csID),
		newEntryDestroyCmd(opts, &csID),
		newEntryOrphanCmd(opts, &csID),
		newEntryRemoveCmd(opts, &csID),
	)
	return cmd
}

func newEntryCreateCmd(opts *client.Options, csID *string) *cobra.Command {
	var (
		catalogItemName string
		ref             string
		valuesJSON      string
		dependsOn       []string
		description     string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Stage a CREATE entry for a new component",
		Long: `Add a CREATE entry to an OPEN change set. The component is materialized at
the given name when the change set deploys.

Names must be lowercase letters, digits, and hyphens; start with a letter;
maximum 63 characters.`,
		Example: `  admiral changeset entry create bootstrap --changeset cs-... --catalog-item datalift-hub-bootstrap \
    --values '{"region":"us-east-1"}'`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}
			if catalogItemName == "" {
				return cmderr.Usage("--catalog-item is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resolvedCatalogItemID, err := resolve.CatalogItem(cmd.Context(), c.Catalog(), catalogItemName)
			if err != nil {
				return err
			}

			req := &changesetv1.SetEntryRequest{
				ChangeSetId:   *csID,
				ComponentName: args[0],
				ChangeType:    changesetv1.ChangeSetEntryType_CHANGE_SET_ENTRY_TYPE_CREATE,
				CatalogItemId: &resolvedCatalogItemID,
				DependsOn:     dependsOn,
			}
			if ref != "" {
				req.Ref = &ref
			}
			if valuesJSON != "" {
				req.ValuesTemplate = &valuesJSON
			}
			if description != "" {
				req.Description = &description
			}

			resp, err := c.ChangeSet().SetEntry(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, entryTable.Render(p, resp.Entry))
		},
	}
	cmd.Flags().StringVar(&catalogItemName, "catalog-item", "", "catalog item name or ID (required)")
	cmd.Flags().StringVar(&ref, "ref", "", "git ref / version to deploy (defaults to the catalog item's ref)")
	cmd.Flags().StringVar(&valuesJSON, "values", "", "values template as a JSON object")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "explicit dependency (component name, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "entry description")
	return cmd
}

func newEntryUpdateCmd(opts *client.Options, csID *string) *cobra.Command {
	var (
		catalogItemName string
		ref             string
		valuesJSON      string
		dependsOn       []string
		description     string
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Stage an UPDATE entry for an existing component",
		Long:  `Add an UPDATE entry to an OPEN change set. Patches the component's HEAD on successful deploy with the non-empty fields below.`,
		Example: `  admiral changeset entry update bootstrap --changeset cs-... --ref v1.2.3
  admiral changeset entry update bootstrap --changeset cs-... --values '{"region":"us-west-2"}'`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			req := &changesetv1.SetEntryRequest{
				ChangeSetId:   *csID,
				ComponentName: args[0],
				ChangeType:    changesetv1.ChangeSetEntryType_CHANGE_SET_ENTRY_TYPE_UPDATE,
				DependsOn:     dependsOn,
			}
			if catalogItemName != "" {
				resolvedCatalogItemID, err := resolve.CatalogItem(cmd.Context(), c.Catalog(), catalogItemName)
				if err != nil {
					return err
				}
				req.CatalogItemId = &resolvedCatalogItemID
			}
			if ref != "" {
				req.Ref = &ref
			}
			if valuesJSON != "" {
				req.ValuesTemplate = &valuesJSON
			}
			if description != "" {
				req.Description = &description
			}

			resp, err := c.ChangeSet().SetEntry(cmd.Context(), req)
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, entryTable.Render(p, resp.Entry))
		},
	}
	cmd.Flags().StringVar(&catalogItemName, "catalog-item", "", "catalog item name or ID to switch to")
	cmd.Flags().StringVar(&ref, "ref", "", "git ref / version to deploy")
	cmd.Flags().StringVar(&valuesJSON, "values", "", "values template as a JSON object")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "explicit dependency (component name, repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "entry description")
	return cmd
}

func newEntryDestroyCmd(opts *client.Options, csID *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <name>",
		Short: "Stage a DESTROY entry for a component",
		Long:  `Add a DESTROY entry to an OPEN change set. The deploy will run the engine's destroy verb (terraform destroy / helm uninstall / kubectl delete) for the component and clear its outputs.`,
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().SetEntry(cmd.Context(), &changesetv1.SetEntryRequest{
				ChangeSetId:   *csID,
				ComponentName: args[0],
				ChangeType:    changesetv1.ChangeSetEntryType_CHANGE_SET_ENTRY_TYPE_DESTROY,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, entryTable.Render(p, resp.Entry))
		},
	}
	return cmd
}

func newEntryOrphanCmd(opts *client.Options, csID *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orphan <name>",
		Short: "Stage an ORPHAN entry for a component",
		Long:  `Add an ORPHAN entry to an OPEN change set. On successful deploy the component is detached from this environment without running terraform destroy; state is preserved.`,
		Args:  flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			resp, err := c.ChangeSet().SetEntry(cmd.Context(), &changesetv1.SetEntryRequest{
				ChangeSetId:   *csID,
				ComponentName: args[0],
				ChangeType:    changesetv1.ChangeSetEntryType_CHANGE_SET_ENTRY_TYPE_ORPHAN,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintResource(resp, entryTable.Render(p, resp.Entry))
		},
	}
	return cmd
}

func newEntryRemoveCmd(opts *client.Options, csID *string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove an entry from an OPEN change set",
		Long:    `Drop a component's entry from an OPEN change set. Does NOT mark the component for destruction; for that, use 'changeset entry destroy'.`,
		Args:    flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if *csID == "" {
				return cmderr.Usage("--changeset is required")
			}

			if err := input.Confirm(cmd, force, "Remove entry "+args[0]+" from change set "+*csID); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			if _, err := c.ChangeSet().RemoveEntry(cmd.Context(), &changesetv1.RemoveEntryRequest{
				ChangeSetId:   *csID,
				ComponentName: args[0],
			}); err != nil {
				return err
			}

			output.Writef(cmd.OutOrStdout(), "Removed entry %s from change set %s\n", args[0], *csID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}
