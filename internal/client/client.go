package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
)

// created records whether this process built a client, so post-run work
// that only matters after network use (proactive session refresh) can skip
// commands like `config list` or `completion`.
var created atomic.Bool

// Options hold the configuration shared across all commands.
type Options struct {
	ServerAddr   string
	Insecure     bool
	PlainText    bool
	Verbose      bool
	ConfigDir    string
	OutputFormat output.Format

	// OIDC settings used by `admiral auth login`.
	Issuer   string
	ClientID string

	// Timeout bounds each RPC. Zero means DefaultTimeout.
	Timeout time.Duration
}

// CreateClient resolves the active credential and builds a client.
//
// API keys go through the SDK client: sent with the "Token" scheme and
// format-checked before dialing. Login sessions are JWTs sent with the
// "Bearer" scheme over a CLI-owned connection whose credential can be
// swapped, so a 401 triggers a refresh that every later call benefits from.
//
// Every call gets a deadline (opts.Timeout, default DefaultTimeout) and
// read-only calls are retried on Unavailable.
func CreateClient(ctx context.Context, opts *Options) (sdkclient.AdmiralClient, error) {
	created.Store(true)
	cred, err := credentials.ResolveToken(opts.ConfigDir)
	if err != nil {
		return nil, err
	}

	mode := modeFor(opts)
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Outermost first: the deadline spans every retry below it.
	interceptors := []grpc.UnaryClientInterceptor{deadlineInterceptor(timeout)}

	if cred.Source == credentials.SourceSession {
		tok := newBearerToken(cred.Token, mode.plaintext())
		interceptors = append(interceptors,
			authRetryInterceptor(opts.ConfigDir, tok.set),
			unavailableRetryInterceptor(),
			debugLogInterceptor(),
		)
		slog.Debug("connecting", "target", opts.ServerAddr, "auth", "session", "transport", mode)
		return newSessionClient(opts.ServerAddr, tok, mode, interceptors)
	}

	interceptors = append(interceptors, unavailableRetryInterceptor(), debugLogInterceptor())
	slog.Debug("connecting", "target", opts.ServerAddr, "auth", "api-key", "transport", mode)
	cfg := sdkclient.Config{
		HostPort:   opts.ServerAddr,
		AuthToken:  cred.Token,
		AuthScheme: cred.AuthScheme,
		ConnectionOptions: sdkclient.ConnectionOptions{
			// The SDK's Insecure means plaintext; certificate verification
			// is controlled through TLSConfig.
			Insecure:  mode.plaintext(),
			TLSConfig: mode.tlsConfig(),
			DialOptions: append([]grpc.DialOption{
				grpc.WithChainUnaryInterceptor(interceptors...),
			}, testDialOptions...),
		},
	}
	if opts.Verbose {
		cfg.Logger = sdkclient.NewSlogLogger(slog.Default())
	}

	c, err := sdkclient.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.ServerAddr, err)
	}
	return c, nil
}

// Created reports whether CreateClient ran during this process.
func Created() bool { return created.Load() }
