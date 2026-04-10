package client

import (
	"context"
	"fmt"
	"log/slog"

	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
)

// Options hold the configuration shared across all commands.
type Options struct {
	ServerAddr   string
	Insecure     bool
	PlainText    bool
	Verbose      bool
	ConfigDir    string
	OutputFormat output.Format

	// OIDC settings (used by auth commands)
	Issuer   string
	ClientID string
	Scopes   []string
}

// CreateClient creates a new AdmiralClient using the SDK.
func CreateClient(_ context.Context, opts *Options) (sdkclient.AdmiralClient, error) {
	result, err := credentials.ResolveToken(opts.ConfigDir)
	if err != nil {
		return nil, err
	}

	insecure := opts.Insecure || opts.PlainText

	cfg := sdkclient.Config{
		HostPort:   opts.ServerAddr,
		AuthToken:  result.Token,
		AuthScheme: sdkclient.AuthSchemeBearer,
		ConnectionOptions: sdkclient.ConnectionOptions{
			Insecure: insecure,
		},
	}

	if opts.Verbose {
		cfg.Logger = sdkclient.NewSlogLogger(slog.Default())
	}

	c, err := sdkclient.New(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.ServerAddr, err)
	}

	return c, nil
}
