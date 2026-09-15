package client

import (
	"crypto/tls"

	"google.golang.org/grpc/credentials"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
)

// transportMode is how the connection is secured. The two flags that select
// it are distinct promises and must stay distinct: --plaintext means no TLS
// at all, --insecure means TLS with the server certificate unverified.
type transportMode int

const (
	transportTLS        transportMode = iota // verify the server certificate (default)
	transportSkipVerify                      // TLS, but accept any certificate (--insecure)
	transportPlaintext                       // no TLS (--plaintext)
)

func modeFor(opts *Options) transportMode {
	switch {
	case opts.PlainText:
		return transportPlaintext
	case opts.Insecure:
		return transportSkipVerify
	default:
		return transportTLS
	}
}

// tlsConfig returns the TLS configuration for a mode, or nil for plaintext.
func (m transportMode) tlsConfig() *tls.Config {
	switch m {
	case transportPlaintext:
		return nil
	case transportSkipVerify:
		return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // that is what --insecure asks for
	default:
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
}

// credentials returns the gRPC transport credentials for a mode.
func (m transportMode) credentials() credentials.TransportCredentials {
	if cfg := m.tlsConfig(); cfg != nil {
		return credentials.NewTLS(cfg)
	}
	return insecurecreds.NewCredentials()
}

func (m transportMode) String() string {
	switch m {
	case transportPlaintext:
		return "plaintext"
	case transportSkipVerify:
		return "tls (unverified)"
	default:
		return "tls"
	}
}

// plaintext reports whether the token travels unencrypted, which is the only
// case where per-RPC credentials may skip the transport-security check.
func (m transportMode) plaintext() bool { return m == transportPlaintext }
