package scopes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	got, err := Validate([]string{"app:read,env:read", "app:read", "run:read"})
	require.NoError(t, err)
	require.Equal(t, []string{"app:read", "env:read", "run:read"}, got, "comma-split, deduplicated, order kept")

	_, err = Validate([]string{"app:*"})
	require.Error(t, err, "scopes attenuate; there are no family wildcards")

	got, err = Validate(nil)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = Validate([]string{"app-read"})
	require.ErrorContains(t, err, `unknown scope "app-read"`)
	require.ErrorContains(t, err, "app:read")

	_, err = Validate([]string{"openid"})
	require.Error(t, err, "identity scopes are not user-selectable")

	_, err = Validate([]string{"agent:exec"})
	require.Error(t, err, "agent-only scope is not assignable to a person")
}

func TestUser(t *testing.T) {
	all := User()
	require.Contains(t, all, "app:read")
	require.NotContains(t, all, "app:*", "no wildcards; scopes are concrete attenuations")
	require.NotContains(t, all, "agent:exec", "agent-only scope is not assignable to a person")
	require.IsIncreasing(t, all)
}
