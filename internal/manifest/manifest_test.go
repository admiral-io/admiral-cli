package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	m, err := Parse([]byte(`
components:
  - path: modules/agent
  - path: ./modules/network_hub/
    name: network-hub
`))
	require.NoError(t, err)
	require.Len(t, m.Components, 2)
	assert.Equal(t, Component{Path: "modules/agent", Name: "agent"}, m.Components[0])
	assert.Equal(t, Component{Path: "modules/network_hub", Name: "network-hub"}, m.Components[1])
}

func TestParseRefuses(t *testing.T) {
	cases := map[string]string{
		"underscore-without-name": "components:\n  - path: modules/network_hub\n",
		"escapes":                 "components:\n  - path: ../elsewhere\n",
		"absolute":                "components:\n  - path: /etc\n",
		"duplicate-path":          "components:\n  - path: a\n  - path: ./a\n",
		"duplicate-name":          "components:\n  - path: x/agent\n  - path: y/agent\n",
		"empty":                   "components: []\n",
		"unknown-field":           "components:\n  - path: a\n    version: 1.0.0\n",
		"no-path":                 "components:\n  - name: a\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), Filename)
		})
	}
}
