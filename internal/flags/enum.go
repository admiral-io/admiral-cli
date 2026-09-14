package flags

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
)

// enumValue is a pflag.Value restricted to a fixed set of strings. An
// invalid value is a usage error naming the accepted set (exit 2).
type enumValue struct {
	dest    *string
	allowed []string
}

func (e *enumValue) String() string { return *e.dest }
func (e *enumValue) Type() string   { return "string" }

func (e *enumValue) Set(v string) error {
	if slices.Contains(e.allowed, v) {
		*e.dest = v
		return nil
	}
	return cmderr.Usage("must be one of %s", strings.Join(e.allowed, ", "))
}

// Enum registers a string flag that only accepts one of allowed. The usage
// text is suffixed with the accepted values so help and errors agree.
func Enum(cmd *cobra.Command, dest *string, name, def, usage string, allowed ...string) {
	*dest = def
	u := fmt.Sprintf("%s: %s", usage, strings.Join(allowed, ", "))
	if def == "" {
		cmd.Flags().Var(&enumValue{dest: dest, allowed: allowed}, name, u)
		return
	}
	cmd.Flags().Var(&enumValue{dest: dest, allowed: allowed}, name, u+" (default "+def+")")
}
