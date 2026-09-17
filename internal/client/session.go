package client

import (
	"context"
	"fmt"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	healthcheckv1 "go.admiral.io/sdk/proto/admiral/api/healthcheck/v1"
	invitationv1 "go.admiral.io/sdk/proto/admiral/api/invitation/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
	tenantv1 "go.admiral.io/sdk/proto/admiral/api/tenant/v1"
	userv1 "go.admiral.io/sdk/proto/admiral/api/user/v1"
)

// bearerToken is a per-RPC credential whose token can be replaced after a
// refresh. Every call reads the current value, so a refreshed session takes
// effect for the rest of the process, not just the retried call.
//
// This exists because grpc-go appends every connection-level credential to
// every call. The SDK client attaches a fixed token at dial time, and adding
// a second credential on retry would send two authorization headers. For
// sessions the CLI therefore dials its own connection with this credential.
type bearerToken struct {
	token     atomic.Pointer[string]
	plaintext bool
}

func newBearerToken(tok string, plaintext bool) *bearerToken {
	b := &bearerToken{plaintext: plaintext}
	b.token.Store(&tok)
	return b
}

func (b *bearerToken) set(tok string) { b.token.Store(&tok) }

func (b *bearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": sdkclient.AuthSchemeBearer.String() + " " + *b.token.Load()}, nil
}

func (b *bearerToken) RequireTransportSecurity() bool { return !b.plaintext }

var _ credentials.PerRPCCredentials = (*bearerToken)(nil)

// sessionClient implements sdkclient.AdmiralClient over a CLI-owned connection.
type sessionClient struct {
	conn *grpc.ClientConn
}

// newSessionClient dials hostPort with a swappable bearer credential and the
// given interceptors, and exposes the result through the SDK's client
// interface so commands cannot tell the two paths apart.
func newSessionClient(hostPort string, tok *bearerToken, mode transportMode, interceptors []grpc.UnaryClientInterceptor) (sdkclient.AdmiralClient, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(mode.credentials()),
		grpc.WithPerRPCCredentials(tok),
		grpc.WithUserAgent(sdkclient.ClientUserAgent()),
		grpc.WithChainUnaryInterceptor(interceptors...),
	}
	opts = append(opts, testDialOptions...)

	conn, err := grpc.NewClient(hostPort, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", hostPort, err)
	}
	return &sessionClient{conn: conn}, nil
}

func (c *sessionClient) Agent() agentv1.AgentAPIClient { return agentv1.NewAgentAPIClient(c.conn) }

func (c *sessionClient) AgentRuntime() agentv1.AgentRuntimeAPIClient {
	return agentv1.NewAgentRuntimeAPIClient(c.conn)
}

func (c *sessionClient) Application() applicationv1.ApplicationAPIClient {
	return applicationv1.NewApplicationAPIClient(c.conn)
}

func (c *sessionClient) ChangeSet() changesetv1.ChangeSetAPIClient {
	return changesetv1.NewChangeSetAPIClient(c.conn)
}

func (c *sessionClient) Credential() credentialv1.CredentialAPIClient {
	return credentialv1.NewCredentialAPIClient(c.conn)
}

func (c *sessionClient) Environment() environmentv1.EnvironmentAPIClient {
	return environmentv1.NewEnvironmentAPIClient(c.conn)
}

func (c *sessionClient) Healthcheck() healthcheckv1.HealthcheckAPIClient {
	return healthcheckv1.NewHealthcheckAPIClient(c.conn)
}

func (c *sessionClient) Invitation() invitationv1.InvitationAPIClient {
	return invitationv1.NewInvitationAPIClient(c.conn)
}

func (c *sessionClient) Registry() registryv1.RegistryAPIClient {
	return registryv1.NewRegistryAPIClient(c.conn)
}

func (c *sessionClient) Run() runv1.RunAPIClient { return runv1.NewRunAPIClient(c.conn) }

func (c *sessionClient) Source() sourcev1.SourceAPIClient { return sourcev1.NewSourceAPIClient(c.conn) }

func (c *sessionClient) Tenant() tenantv1.TenantAPIClient { return tenantv1.NewTenantAPIClient(c.conn) }

func (c *sessionClient) User() userv1.UserAPIClient { return userv1.NewUserAPIClient(c.conn) }

func (c *sessionClient) ValidateToken() error { return nil } // JWTs are validated by the server

func (c *sessionClient) Version() string { return sdkclient.Version() }

func (c *sessionClient) Close() error { return c.conn.Close() }
