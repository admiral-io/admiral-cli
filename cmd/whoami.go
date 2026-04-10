package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
	userv1 "go.admiral.io/sdk/proto/admiral/user/v1"
)

func newWhoamiCmd(opts *client.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current user, organization, and session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck // best-effort cleanup

			resp, err := c.User().GetMe(cmd.Context(), &userv1.GetMeRequest{})
			if err != nil {
				return fmt.Errorf("failed to get user info: %w", err)
			}

			user := resp.GetUser()

			details := []output.Detail{
				{Key: "Email", Value: user.GetEmail()},
				{Key: "Display Name", Value: user.GetDisplayName()},
				{Key: "ID", Value: user.GetId()},
				{Key: "Server", Value: opts.ServerAddr},
			}

			p := output.NewPrinter(opts.OutputFormat)

			sections := []output.Section{
				{Details: details},
			}

			return p.PrintDetail(resp, sections)
		},
	}
}
