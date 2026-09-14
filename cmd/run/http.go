package run

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/credentials"
)

// phaseOutputURL builds the HTTP URL for the phase-keyed transcript
// endpoint. runID accepts either a UUID or a run-<suffix> display ID --
// the server resolves either form via GetByIdentifier. phase is "plan"
// or "apply".
func phaseOutputURL(opts *client.Options, runID, revisionID, phase string) string {
	scheme := "https"
	if opts.PlainText {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/runs/%s/revisions/%s/%s",
		scheme, opts.ServerAddr, runID, revisionID, phase)
}

// addAuth attaches a Bearer authorization header to the request using the
// CLI's resolved token. Mirrors cmd/state/http.go's helper; duplicated here
// rather than extracted because two call sites do not yet warrant a shared
// internal/http package.
func addAuth(req *http.Request, opts *client.Options) {
	result, err := credentials.ResolveToken(opts.ConfigDir)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+result.Token)
}

// httpClient returns an HTTP client whose TLS verification mirrors the
// shared --insecure flag.
func httpClient(opts *client.Options) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &http.Client{Transport: transport}
}
