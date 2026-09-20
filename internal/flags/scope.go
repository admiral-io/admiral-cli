package flags

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
)

// App registers --app on cmd with shell completion of application names.
// Scope has no environment-variable or config default (style guide §1.2):
// it comes from the line, as a flag or as the app/env path.
func App(cmd *cobra.Command, dest *string, opts *client.Options) {
	cmd.Flags().StringVar(dest, "app", "", "application name or ID")
	complete.Flag(cmd, "app", complete.Apps(opts))
}

// Env registers --env on cmd with shell completion of environment names.
// The value is a name or ID, or an app/env path; see EnvTarget.
func Env(cmd *cobra.Command, dest *string, opts *client.Options) {
	cmd.Flags().StringVar(dest, "env", "", "environment name or ID, or app/env")
	complete.Flag(cmd, "env", complete.Envs(opts))
}

// ScopeTarget picks the application and environment a collection command
// is scoped to. The scope is named as the command's own positional
// argument — an application (shop) or a path (shop/prod) — or with --app
// and --env, never both: an explicit flag beside the argument is a usage
// error even when the two agree, the same rule EnvTarget enforces for a
// path (style guide §1.2). args is the command's positional arguments, of
// which at most the first names the scope.
//
// Both results are empty when nothing was given; the caller decides what
// an unscoped listing means for its own collection.
func ScopeTarget(cmd *cobra.Command, app, env string, args []string) (string, string, error) {
	if len(args) == 0 {
		return EnvTarget(cmd, app, env)
	}
	for _, name := range []string{"app", "env"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			return "", "", cmderr.UsageHint("Name the scope as the argument or with --app/--env, not both.",
				"--%s cannot be combined with a scope argument", name)
		}
	}
	target := args[0]
	if !strings.Contains(target, "/") {
		return target, "", nil
	}
	// A path scopes to one environment. EnvTarget validates its shape, and
	// the flags are known unset, so it cannot report a conflict here.
	return EnvTarget(cmd, "", target)
}

// EnvTarget splits what the user typed for an environment into its
// application and environment parts. target is an environment name or ID
// (scoped by app, the --app flag's value) or an app/env path, which
// carries its own scope. Path or flag, never both: a path beside an
// explicit --app is a usage error even when the two agree (style guide
// §1.2). Names cannot contain "/", so the split is unambiguous.
func EnvTarget(cmd *cobra.Command, app, target string) (string, string, error) {
	pathApp, name, isPath := strings.Cut(target, "/")
	if !isPath {
		return app, target, nil
	}
	if f := cmd.Flags().Lookup("app"); f != nil && f.Changed {
		return "", "", cmderr.UsageHint("Give the environment as app/env or with --app, not both.",
			"--app cannot be combined with a path")
	}
	if pathApp == "" || name == "" || strings.Contains(name, "/") {
		return "", "", cmderr.Usage("invalid environment %q: expected app/env", target)
	}
	return pathApp, name, nil
}
