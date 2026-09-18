package bundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAmbientCredentials(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".tofurc"), []byte(
		"credentials \"app.terraform.io\" {\n  token = \"from-rc\"\n}\n"+
			"credentials \"registry.acme-corp.example\" {\n  token = \"acme-rc\"\n}\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".terraform.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".terraform.d", "credentials.tfrc.json"), []byte(
		`{"credentials": {"login.example": {"token": "from-login"}}}`), 0o644))

	a := &AmbientCredentials{home: home, environ: []string{
		"TF_TOKEN_registry_acme__corp_example=acme-env",
		"tf_token_lower_example=lower",
		"TF_TOKEN_empty_example=",
	}}
	lookup := func(u string) string {
		c, err := a.Lookup(context.Background(), u)
		require.NoError(t, err)
		if c == nil {
			return ""
		}
		return c.Token
	}
	assert.Equal(t, "from-rc", lookup("https://app.terraform.io/"))
	assert.Equal(t, "from-rc", lookup("https://APP.terraform.io/v1/modules/"))
	assert.Equal(t, "acme-env", lookup("https://registry.acme-corp.example/"), "the environment wins over the rc file")
	assert.Equal(t, "lower", lookup("https://lower.example/"), "the variable name is matched case-insensitively")
	assert.Equal(t, "from-login", lookup("https://login.example/"), "tofu login's file is read too")
	assert.Equal(t, "", lookup("https://empty.example/"))
	assert.Equal(t, "", lookup("https://registry.opentofu.org/"))
}
