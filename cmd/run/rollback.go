package run

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/input"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newRollbackCmd(opts *client.Options) *cobra.Command {
	var (
		appName string
		envName string
		message string
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "rollback <run-id>",
		Short: "Roll back to a prior run",
		Long: `Create a new run that re-plans and re-applies the configuration from a
previous successful run. The rollback goes through the same plan/approve/apply
cycle as a normal run, producing a Terraform plan that shows the diff from
current state to the prior configuration.`,
		Example: `  # Into the run's own environment
  admiral run rollback <run-id>

  # Into a named environment
  admiral run rollback <run-id> --env billing/staging
  admiral run rollback <run-id> --env billing/staging -m "reverting bad change"`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceRunID := args[0]

			app, env, err := flags.EnvTarget(cmd, appName, envName)
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			// If app/env weren't provided, resolve them from the source run.
			var resolvedAppID, resolvedEnvID string
			if app == "" || env == "" {
				runResp, err := c.Run().GetRun(cmd.Context(), &runv1.GetRunRequest{
					RunId: sourceRunID,
				})
				if err != nil {
					return fmt.Errorf("fetching source run: %w", err)
				}
				if app == "" {
					resolvedAppID = runResp.Run.ApplicationId
				}
				if env == "" {
					resolvedEnvID = runResp.Run.EnvironmentId
				}
			}

			if resolvedAppID == "" {
				resolvedAppID, err = resolve.App(cmd.Context(), c.Application(), app)
				if err != nil {
					return err
				}
			}
			if resolvedEnvID == "" {
				resolvedEnvID, err = resolve.Environment(cmd.Context(), c.Environment(), c.Application(), app, env)
				if err != nil {
					return err
				}
			}

			if err := input.Confirm(cmd, force,
				fmt.Sprintf("Re-apply the configuration from run %s as a new run", sourceRunID)); err != nil {
				return err
			}

			resp, err := c.Run().CreateRun(cmd.Context(), &runv1.CreateRunRequest{
				ApplicationId: resolvedAppID,
				EnvironmentId: resolvedEnvID,
				Message:       message,
				SourceRunId:   sourceRunID,
			})
			if err != nil {
				return err
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.Run, RunID(resp.Run), RunTable.Render(p, resp.Run))
		},
	}

	flags.App(cmd, &appName, opts)
	flags.Env(cmd, &envName, opts)
	cmd.Flags().StringVarP(&message, "message", "m", "", "run message")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")

	return cmd
}
