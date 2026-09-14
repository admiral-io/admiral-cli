package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	credstore "go.admiral.io/cli/internal/credentials"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
)

func TestTransportMode(t *testing.T) {
	require.Equal(t, transportTLS, modeFor(&Options{}))
	require.Equal(t, transportSkipVerify, modeFor(&Options{Insecure: true}))
	require.Equal(t, transportPlaintext, modeFor(&Options{PlainText: true}))
	require.Equal(t, transportPlaintext, modeFor(&Options{PlainText: true, Insecure: true}), "plaintext wins; there is no certificate to skip")

	require.Nil(t, transportPlaintext.tlsConfig())
	require.False(t, transportTLS.tlsConfig().InsecureSkipVerify)
	require.True(t, transportSkipVerify.tlsConfig().InsecureSkipVerify)
	require.Equal(t, uint16(tls.VersionTLS12), transportSkipVerify.tlsConfig().MinVersion, "--insecure still negotiates real TLS")

	require.Equal(t, "insecure", transportPlaintext.credentials().Info().SecurityProtocol)
	require.Equal(t, "tls", transportSkipVerify.credentials().Info().SecurityProtocol)
	require.Equal(t, "tls", transportTLS.credentials().Info().SecurityProtocol)
}

// selfSignedTLS returns server credentials for a certificate no client
// trusts, the shape of a staging box with a homemade cert.
func selfSignedTLS(t *testing.T) credentials.TransportCredentials {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "staging.test"},
		DNSNames:     []string{"staging.test", "bufconn"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
}

// startTLSServer serves fakeAPI over TLS on bufconn.
func startTLSServer(t *testing.T, api *fakeAPI) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.Creds(selfSignedTLS(t)))
	applicationv1.RegisterApplicationAPIServer(srv, api)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	prev := testDialOptions
	testDialOptions = []grpc.DialOption{grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	})}
	t.Cleanup(func() { testDialOptions = prev })
}

// Each credential path, in each transport mode, against a self-signed
// server: only --insecure gets through, and it does so over TLS.
func TestCreateClient_TransportModes(t *testing.T) {
	paths := map[string]func(t *testing.T) *Options{
		"api-key via SDK": func(t *testing.T) *Options {
			t.Setenv(credstore.EnvAPIKey, testToken)
			return &Options{ServerAddr: "bufconn:1", ConfigDir: t.TempDir()}
		},
		"session via CLI conn": func(t *testing.T) *Options {
			os.Unsetenv(credstore.EnvAPIKey)
			dir := t.TempDir()
			require.NoError(t, credstore.Save(dir, &credstore.Credential{
				Kind: credstore.KindSession, AccessToken: "jwt", Expiry: time.Now().Add(time.Hour),
			}))
			return &Options{ServerAddr: "passthrough:///bufconn", ConfigDir: dir}
		},
	}

	for name, mk := range paths {
		t.Run(name, func(t *testing.T) {
			t.Run("default verifies and refuses the self-signed cert", func(t *testing.T) {
				api := &fakeAPI{script: map[string][]error{}}
				startTLSServer(t, api)
				opts := mk(t)
				c, err := CreateClient(context.Background(), opts)
				require.NoError(t, err)
				defer c.Close() //nolint:errcheck
				_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
				require.Equal(t, codes.Unavailable, status.Code(err))
				require.Contains(t, err.Error(), "certificate")
				require.Empty(t, api.calls, "no request reaches the server")
			})
			t.Run("--insecure connects over TLS without verifying", func(t *testing.T) {
				api := &fakeAPI{script: map[string][]error{}}
				startTLSServer(t, api)
				opts := mk(t)
				opts.Insecure = true
				c, err := CreateClient(context.Background(), opts)
				require.NoError(t, err)
				defer c.Close() //nolint:errcheck
				_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
				require.NoError(t, err)
				require.Len(t, api.calls, 1)
			})
			t.Run("--plaintext cannot talk to a TLS server", func(t *testing.T) {
				api := &fakeAPI{script: map[string][]error{}}
				startTLSServer(t, api)
				opts := mk(t)
				opts.PlainText = true
				c, err := CreateClient(context.Background(), opts)
				require.NoError(t, err)
				defer c.Close() //nolint:errcheck
				_, err = c.Application().ListApplications(context.Background(), &applicationv1.ListApplicationsRequest{})
				require.Error(t, err, "a plaintext client must not silently succeed against TLS")
				require.Empty(t, api.calls)
			})
		})
	}
}
