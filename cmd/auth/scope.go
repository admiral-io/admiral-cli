package auth

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.admiral.io/sdk/proto/admiral/scopes"
)

// userScopes returns the resource scopes a person may attenuate a session
// to: every catalog scope assignable to a user credential. Scopes are a
// reduction of the principal's permissions, so there are no wildcards to
// offer; a scope that implies others (write implies read) covers them.
func userScopes() []string {
	var out []string
	for name, sc := range scopes.Catalog {
		if slices.Contains(sc.AssignableTo, scopes.TokenTypePAT) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// validateScopes rejects anything that is not a known user scope, so a typo
// fails at login instead of producing a session with more or less access
// than intended. Accepts comma-separated values within a single flag.
func validateScopes(raw []string) ([]string, error) {
	allowed := userScopes()
	var out []string
	for _, item := range raw {
		for _, s := range strings.Split(item, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !slices.Contains(allowed, s) {
				return nil, fmt.Errorf("unknown scope %q; valid scopes: %s", s, strings.Join(allowed, ", "))
			}
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out, nil
}
