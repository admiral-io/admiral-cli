package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Credentials answers what a fetch may present to a URL. The platform
// registers credentials against a host or URL prefix and resolves them by
// longest prefix (design section 6); this is that lookup, so the fetcher is
// the same code whether the answers come from a developer's machine or from
// registered sources. A nil credential is an anonymous fetch.
type Credentials interface {
	Lookup(ctx context.Context, rawURL string) (*Credential, error)
}

// Credential is what a fetch presents: a bearer token for a registry or an
// archive host.
type Credential struct {
	Token string
}

// AmbientCredentials reads what tofu itself reads for a private module
// registry: `TF_TOKEN_<host>` in the environment, then the `credentials`
// blocks of the CLI configuration (TF_CLI_CONFIG_FILE, ~/.tofurc,
// ~/.terraformrc) and the file `tofu login` writes. Git and http archives are
// not answered here: git has its own agent, helper and insteadOf machinery,
// and an archive URL carries any auth it has.
type AmbientCredentials struct {
	environ []string
	home    string
}

// NewAmbientCredentials reads the process environment and home directory.
func NewAmbientCredentials() *AmbientCredentials {
	home, _ := os.UserHomeDir()
	return &AmbientCredentials{environ: os.Environ(), home: home}
}

// Lookup returns the token configured for the URL's host, or nil.
func (a *AmbientCredentials) Lookup(_ context.Context, rawURL string) (*Credential, error) {
	host := hostOf(rawURL)
	if host == "" {
		return nil, nil
	}
	if tok := a.fromEnv(host); tok != "" {
		return &Credential{Token: tok}, nil
	}
	tok, err := a.fromConfig(host)
	if err != nil {
		return nil, err
	}
	if tok != "" {
		return &Credential{Token: tok}, nil
	}
	return nil, nil
}

// fromEnv finds TF_TOKEN_<host>, with `.` as `_` and `-` as `__`, matched
// case-insensitively the way tofu matches it.
func (a *AmbientCredentials) fromEnv(host string) string {
	want := "TF_TOKEN_" + strings.NewReplacer("-", "__", ".", "_").Replace(host)
	for _, kv := range a.environ {
		name, value, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(name, want) && value != "" {
			return value
		}
	}
	return ""
}

// fromConfig reads the first CLI configuration that exists, then the
// credentials file `tofu login` maintains.
func (a *AmbientCredentials) fromConfig(host string) (string, error) {
	var candidates []string
	if p := a.envValue("TF_CLI_CONFIG_FILE"); p != "" {
		candidates = append(candidates, p)
	} else if a.home != "" {
		candidates = append(candidates, filepath.Join(a.home, ".tofurc"), filepath.Join(a.home, ".terraformrc"))
	}
	for _, p := range candidates {
		src, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		tok, err := credentialsFromHCL(src, p, host)
		if err != nil {
			return "", err
		}
		if tok != "" {
			return tok, nil
		}
		break // tofu reads one configuration file, the first that exists
	}
	if a.home == "" {
		return "", nil
	}
	for _, dir := range []string{".terraform.d", ".tofu.d"} {
		p := filepath.Join(a.home, dir, "credentials.tfrc.json")
		src, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		var f struct {
			Credentials map[string]struct {
				Token string `json:"token"`
			} `json:"credentials"`
		}
		if err := json.Unmarshal(src, &f); err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		for h, c := range f.Credentials {
			if strings.EqualFold(h, host) && c.Token != "" {
				return c.Token, nil
			}
		}
	}
	return "", nil
}

func (a *AmbientCredentials) envValue(name string) string {
	for _, kv := range a.environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// credentialsFromHCL reads `credentials "host" { token = "..." }` blocks.
func credentialsFromHCL(src []byte, filename, host string) (string, error) {
	f, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return "", fmt.Errorf("%s: %s", filename, diags.Error())
	}
	content, _, diags := f.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "credentials", LabelNames: []string{"host"}}},
	})
	if diags.HasErrors() {
		return "", fmt.Errorf("%s: %s", filename, diags.Error())
	}
	for _, b := range content.Blocks {
		if !strings.EqualFold(b.Labels[0], host) {
			continue
		}
		attrs, diags := b.Body.JustAttributes()
		if diags.HasErrors() {
			return "", fmt.Errorf("%s: %s", filename, diags.Error())
		}
		tok, ok := attrs["token"]
		if !ok {
			continue
		}
		v, diags := tok.Expr.Value(nil)
		if diags.HasErrors() || v.IsNull() || v.Type() != cty.String {
			continue
		}
		if s := v.AsString(); s != "" {
			return s, nil
		}
	}
	return "", nil
}

// hostOf is the host part of a URL, or the string itself when it is a bare
// hostname.
func hostOf(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		return strings.ToLower(strings.TrimSuffix(rawURL, "/"))
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
