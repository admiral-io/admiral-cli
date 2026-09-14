package state

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/credentials"
)

// stateURL builds the HTTP URL for the state backend endpoint.
func stateURL(opts *client.Options, componentID, environmentID string) string {
	scheme := "https"
	if opts.PlainText {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/state/%s/env/%s",
		scheme, opts.ServerAddr, componentID, environmentID)
}

// addAuth adds Bearer authentication to the request using the resolved token.
func addAuth(req *http.Request, opts *client.Options) {
	result, err := credentials.ResolveToken(opts.ConfigDir)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+result.Token)
}

// httpClient returns an HTTP client configured for the server connection.
func httpClient(opts *client.Options) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &http.Client{Transport: transport}
}
