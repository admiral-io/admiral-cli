package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"inventory-api":          `'inventory-api'`,
		"":                       `''`,
		"o'neil":                 `'o\'neil'`,
		`x' OR field['id'] != '`: `'x\' OR field[\'id\'] != \''`,
		`back\slash`:             `'back\slash'`,
		`say "hi"`:               `'say "hi"'`,
	}
	for in, want := range cases {
		got, err := Quote(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}

	_, err := Quote(`ends-with\`)
	require.ErrorContains(t, err, "ends with a backslash")
}

func TestEq(t *testing.T) {
	got, err := Eq("name", "o'neil")
	require.NoError(t, err)
	require.Equal(t, `field['name'] = 'o\'neil'`, got)

	got, err = Eq("labels.team's", "a")
	require.NoError(t, err)
	require.Equal(t, `field['labels.team\'s'] = 'a'`, got)

	_, err = Eq("name", `bad\`)
	require.Error(t, err)
}

func TestAnd(t *testing.T) {
	require.Equal(t, "", And())
	require.Equal(t, "a", And("", "a", ""))
	require.Equal(t, "a AND b", And("a", "b"))
}
