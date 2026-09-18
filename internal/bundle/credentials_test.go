package bundle

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

	require.NoError(t, os.WriteFile(filepath.Join(home, "repositories.yaml"), []byte(
		"repositories:\n- name: acme\n  url: https://charts.acme.example/stable\n  username: helm-user\n  password: helm-pw\n- name: public\n  url: https://charts.public.example\n"), 0o644))

	a := &AmbientCredentials{home: home, environ: []string{
		"HELM_REPOSITORY_CONFIG=" + filepath.Join(home, "repositories.yaml"),
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

	c, err := a.Lookup(context.Background(), "https://charts.acme.example/stable/index.yaml")
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, &BasicAuth{Username: "helm-user", Password: "helm-pw"}, c.Basic, "helm repo add --username")
	c, err = a.Lookup(context.Background(), "https://charts.acme.example/other/index.yaml")
	require.NoError(t, err)
	assert.Nil(t, c, "a prefix, not a host")
	c, err = a.Lookup(context.Background(), "https://charts.public.example/index.yaml")
	require.NoError(t, err)
	assert.Nil(t, c, "no username, nothing to present")
}

func TestCredentialFamilies(t *testing.T) {
	ctx := context.Background()
	req := func() *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "https://example.test/x", nil)
		return r
	}

	r := req()
	require.NoError(t, (&Credential{Token: "tok"}).authorize(r))
	assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))

	r = req()
	require.NoError(t, (&Credential{Basic: &BasicAuth{Username: "u", Password: "p"}}).authorize(r))
	u, p, ok := r.BasicAuth()
	assert.True(t, ok)
	assert.Equal(t, []string{"u", "p"}, []string{u, p})

	r = req()
	require.NoError(t, (*Credential)(nil).authorize(r))
	assert.Empty(t, r.Header.Get("Authorization"))

	assert.ErrorIs(t, (&Credential{SSHKey: &SSHKey{PEM: []byte("k")}}).authorize(req()), ErrCredentialFamily, "an ssh key has no HTTP form")

	// git: each family goes to the scheme it fits, through the environment,
	// and is undone afterwards.
	f := newFetcher(t.TempDir(), staticCredentials{
		"ssh://git@github.com/acme/infra": {SSHKey: &SSHKey{PEM: []byte("PEM")}},
		"https://github.com/acme/infra":   {Basic: &BasicAuth{Username: "u", Password: "p"}},
		"https://gitlab.test/acme/infra":  {Token: "tok"},
	}, nil)
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		require.NoError(t, err)
		return u
	}

	undo, err := f.gitEnv(ctx, parse("ssh://git@github.com/acme/infra.git"))
	require.NoError(t, err)
	cmd := os.Getenv("GIT_SSH_COMMAND")
	assert.Contains(t, cmd, "IdentitiesOnly=yes")
	keyFile := strings.Trim(strings.Fields(cmd)[2], `"`)
	pem, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Equal(t, "PEM", string(pem))
	undo()
	assert.Empty(t, os.Getenv("GIT_SSH_COMMAND"))
	assert.NoFileExists(t, keyFile, "the key file does not outlive the fetch")

	undo, err = f.gitEnv(ctx, parse("https://github.com/acme/infra.git"))
	require.NoError(t, err)
	assert.Equal(t, "credential.helper", os.Getenv("GIT_CONFIG_KEY_0"))
	assert.Equal(t, "p", os.Getenv("ADMIRAL_GIT_PASSWORD"))
	undo()
	assert.Empty(t, os.Getenv("GIT_CONFIG_COUNT"))

	undo, err = f.gitEnv(ctx, parse("https://gitlab.test/acme/infra.git"))
	require.NoError(t, err)
	assert.Contains(t, os.Getenv("GIT_CONFIG_VALUE_0"), "x-access-token")
	assert.Equal(t, "tok", os.Getenv("ADMIRAL_GIT_PASSWORD"))
	undo()

	_, err = f.gitEnv(ctx, parse("https://github.com/acme/infra.git?x"))
	require.NoError(t, err)
	f.creds = staticCredentials{"ssh://git@github.com/acme/infra": {Basic: &BasicAuth{Username: "u", Password: "p"}}}
	_, err = f.gitEnv(ctx, parse("ssh://git@github.com/acme/infra.git"))
	assert.ErrorIs(t, err, ErrCredentialFamily, "basic auth cannot ride ssh")
}

// staticCredentials answers by the longest registered prefix, the way the
// platform's registered credentials will.
type staticCredentials map[string]*Credential

func (s staticCredentials) Lookup(_ context.Context, rawURL string) (*Credential, error) {
	var best string
	for prefix := range s {
		if strings.HasPrefix(rawURL, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return nil, nil
	}
	return s[best], nil
}
