// Package scopes is the scope vocabulary a person may choose from, for a
// session (auth login --scope) and for an API key (apikey create --scope).
package scopes

import (
	"slices"
	"sort"
	"strings"

	"go.admiral.io/cli/internal/cmderr"
	sdkscopes "go.admiral.io/sdk/proto/admiral/scopes"
)

// User returns the resource scopes a person may attenuate a session or a
// key to: every catalog scope assignable to a user credential. Scopes are a
// reduction of the principal's permissions, so there are no wildcards to
// offer; a scope that implies others (write implies read) covers them.
func User() []string {
	var out []string
	for name, sc := range sdkscopes.Catalog {
		if slices.Contains(sc.AssignableTo, sdkscopes.TokenTypePAT) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Validate rejects anything that is not a known user scope, so a typo
// fails here instead of producing a session with more or less access
// than intended. Accepts comma-separated values within a single flag.
func Validate(raw []string) ([]string, error) {
	allowed := User()
	var out []string
	for _, item := range raw {
		for _, s := range strings.Split(item, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !slices.Contains(allowed, s) {
				return nil, cmderr.Usage("unknown scope %q; valid scopes: %s", s, strings.Join(allowed, ", "))
			}
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out, nil
}
