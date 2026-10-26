package state

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/structpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	tenantv1 "go.admiral.io/sdk/proto/admiral/api/tenant/v1"
)

type StateCmd struct {
	Cmd *cobra.Command
}

// NewStateCmd reads the history of hosted Terraform state. Terraform itself
// reads and writes the current state; this is what it has no command for.
func NewStateCmd(opts *client.Options) *StateCmd {
	root := &StateCmd{}
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Read the history of hosted Terraform state",
		Long: `Read the history of a Terraform component's hosted state.

Admiral keeps every version of a component's state when its environment has
no backend set. Terraform reads and writes the current one; these commands
list the versions and download one, for a rollback with
'terraform state push -force' or to see what changed.

'versions' names a component by its environment (app/env) and its name;
'download' takes a version id from it.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags.Group(cmd)
	cmd.AddCommand(
		newVersionsCmd(opts),
		newDownloadCmd(opts),
	)
	root.Cmd = cmd
	return root
}

// workspace is the organization and workspace name a component's state is
// served under, `<application>_<environment>_<component>`, the names its
// cloud block uses.
func workspace(ctx context.Context, c sdkclient.AdmiralClient, cmd *cobra.Command, target, component string) (string, string, error) {
	appScope, name, err := flags.EnvTarget(cmd, "", target)
	if err != nil {
		return "", "", err
	}
	envID, err := resolve.Environment(ctx, c.Environment(), c.Application(), appScope, name)
	if err != nil {
		return "", "", err
	}
	env, err := c.Environment().GetEnvironment(ctx, &environmentv1.GetEnvironmentRequest{EnvironmentId: envID})
	if err != nil {
		return "", "", err
	}
	app, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: env.GetEnvironment().GetApplicationId()})
	if err != nil {
		return "", "", err
	}
	tenant, err := c.Tenant().GetTenant(ctx, &tenantv1.GetTenantRequest{})
	if err != nil {
		return "", "", err
	}
	return tenant.GetTenant().GetSlug(), app.GetApplication().GetName() + "_" + env.GetEnvironment().GetName() + "_" + component, nil
}

// asStruct lets the printer render a plain value like an API object.
func asStruct(v any) (*structpb.Struct, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}
