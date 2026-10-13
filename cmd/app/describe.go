package app

import (
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

func newDescribeCmd(opts *client.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "describe <name>",
		Short:             "Show everything about an application",
		Long:              "Show the application's identity, its environments, and its most recent runs.\n\ndescribe is a human view. Use 'app get -o json' for the raw record.",
		Example:           "  # Show an application, its environments, and recent runs\n  admiral app describe billing-api",
		Aliases:           []string{"desc"},
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Apps(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck
			ctx := cmd.Context()

			id, err := resolve.App(ctx, c.Application(), args[0])
			if err != nil {
				return err
			}
			resp, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: id})
			if err != nil {
				return err
			}
			a := resp.Application

			envs, envErr := c.Environment().ListEnvironments(ctx, &environmentv1.ListEnvironmentsRequest{ApplicationId: &id, PageSize: 100})

			// Runs are listed per environment, so the app's runs are those
			// of its environments.
			envNames := map[string]string{}
			runs, runErr := &runv1.ListRunsResponse{}, envErr
			if envErr == nil && len(envs.Environments) > 0 {
				ids := make([]string, 0, len(envs.Environments))
				for _, e := range envs.Environments {
					ids = append(ids, e.Id)
					envNames[e.Id] = e.Name
				}
				byEnv, err := filter.In("environment_id", ids)
				if err != nil {
					return err
				}
				runs, runErr = c.Run().ListRuns(ctx, &runv1.ListRunsRequest{Filter: byEnv, PageSize: 5})
			}

			d := output.NewDescribe()
			d.Field("Name", a.Name)
			d.Field("ID", a.Id)
			d.Field("Description", a.Description)
			d.Fields("Labels", output.LabelLines(a.Labels))
			d.Field("Created", output.FormatDescribeTime(a.CreatedAt))
			d.Field("Created By", output.FormatActor(a.CreatedBy))

			if envErr != nil {
				d.Unavailable("Environments", cmderr.Format(envErr))
			} else {
				envRows := make([][]string, 0, len(envs.Environments))
				for _, e := range envs.Environments {
					envRows = append(envRows, []string{e.Name, e.Description, output.FormatLabels(e.Labels), output.FormatAge(e.CreatedAt)})
				}
				d.Table("Environments", []string{"Name", "Description", "Labels", "Age"}, envRows)
			}

			if runErr != nil {
				d.Unavailable("Recent Runs", cmderr.Format(runErr))
			} else {
				runRows := make([][]string, 0, len(runs.Runs))
				for _, r := range runs.Runs {
					runRows = append(runRows, []string{r.Id, envNames[r.EnvironmentId], output.FormatEnum(r.Status), output.OrEmpty(r.ChangeSetId), output.FormatAge(r.CreatedAt)})
				}
				d.Table("Recent Runs", []string{"ID", "Env", "Status", "Change Set", "Age"}, runRows)
			}

			if status.Code(envErr) == codes.PermissionDenied || status.Code(runErr) == codes.PermissionDenied {
				d.Hint(cmderr.ScopeHint)
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(d, "admiral app get "+a.Name)
		},
	}
	return cmd
}
