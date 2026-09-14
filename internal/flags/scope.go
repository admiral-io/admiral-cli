package flags

import (
	"os"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/complete"
)

// Environment variables that supply parent scope when the flag is omitted.
// Precedence is flag > env > error.
const (
	EnvApp = "ADMIRAL_APP"
	EnvEnv = "ADMIRAL_ENV"
)

// App registers --app on cmd, defaulting from ADMIRAL_APP, with shell
// completion of application names.
func App(cmd *cobra.Command, dest *string, opts *client.Options) {
	cmd.Flags().StringVar(dest, "app", os.Getenv(EnvApp), "application name or ID (default from ADMIRAL_APP)")
	complete.Flag(cmd, "app", complete.Apps(opts))
}

// Env registers --env on cmd, defaulting from ADMIRAL_ENV.
func Env(cmd *cobra.Command, dest *string) {
	cmd.Flags().StringVar(dest, "env", os.Getenv(EnvEnv), "environment name or ID (default from ADMIRAL_ENV)")
}
