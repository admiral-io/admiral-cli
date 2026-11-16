package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	admcreds "go.admiral.io/cli/internal/credentials"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	apikeyv1 "go.admiral.io/sdk/proto/admiral/api/apikey/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	healthcheckv1 "go.admiral.io/sdk/proto/admiral/api/healthcheck/v1"
	integrationv1 "go.admiral.io/sdk/proto/admiral/api/integration/v1"
	invitationv1 "go.admiral.io/sdk/proto/admiral/api/invitation/v1"
	organizationv1 "go.admiral.io/sdk/proto/admiral/api/organization/v1"
	projectv1 "go.admiral.io/sdk/proto/admiral/api/project/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
	serviceaccountv1 "go.admiral.io/sdk/proto/admiral/api/serviceaccount/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
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
//
// It also renews the session before a call once the token is within
// RefreshWindow of expiry. An access token lives 15 minutes and an apply may
// be waited on for hours, so a command that polls must refresh as it goes;
// refreshing only after a 401 left a long wait one rejection from "not
// signed in" while the run carried on.
type bearerToken struct {
	mu        sync.Mutex
	token     string
	expiry    time.Time
	plaintext bool
	// renew returns the stored session, refreshing it when it is near
	// expiry. Nil never renews.
	renew func(context.Context) (*admcreds.TokenResult, error)
}

func newBearerToken(tok *admcreds.TokenResult, plaintext bool, renew func(context.Context) (*admcreds.TokenResult, error)) *bearerToken {
	return &bearerToken{token: tok.Token, expiry: tok.Expiry, plaintext: plaintext, renew: renew}
}

func (b *bearerToken) set(tok *admcreds.TokenResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.token, b.expiry = tok.Token, tok.Expiry
}

// GetRequestMetadata renews a token near expiry before attaching it. A
// refused refresh ends the call as Unauthenticated, which is what the server
// would answer. Any other failure, such as the token endpoint being
// unreachable, sends the current token: it may still be good, and if not the
// server's 401 gets the retry interceptor's one forced refresh.
func (b *bearerToken) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.renew != nil && !b.expiry.IsZero() && time.Until(b.expiry) <= admcreds.RefreshWindow {
		fresh, err := b.renew(ctx)
		switch {
		case err == nil:
			b.token, b.expiry = fresh.Token, fresh.Expiry
		case errors.Is(err, admcreds.ErrSessionExpired):
			return nil, status.Error(codes.Unauthenticated, err.Error())
		default:
			slog.Debug("session renewal failed; using the current token", "error", err)
		}
	}
	return map[string]string{"authorization": sdkclient.AuthSchemeBearer.String() + " " + b.token}, nil
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

func (c *sessionClient) Job() agentv1.JobAPIClient { return agentv1.NewJobAPIClient(c.conn) }

func (c *sessionClient) ApiKey() apikeyv1.ApiKeyAPIClient { return apikeyv1.NewApiKeyAPIClient(c.conn) }

func (c *sessionClient) AgentRuntime() agentv1.AgentRuntimeAPIClient {
	return agentv1.NewAgentRuntimeAPIClient(c.conn)
}

func (c *sessionClient) Project() projectv1.ProjectAPIClient {
	return projectv1.NewProjectAPIClient(c.conn)
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

func (c *sessionClient) Integration() integrationv1.IntegrationAPIClient {
	return integrationv1.NewIntegrationAPIClient(c.conn)
}

func (c *sessionClient) Invitation() invitationv1.InvitationAPIClient {
	return invitationv1.NewInvitationAPIClient(c.conn)
}

func (c *sessionClient) Registry() registryv1.RegistryAPIClient {
	return registryv1.NewRegistryAPIClient(c.conn)
}

func (c *sessionClient) Run() runv1.RunAPIClient { return runv1.NewRunAPIClient(c.conn) }

func (c *sessionClient) Source() sourcev1.SourceAPIClient { return sourcev1.NewSourceAPIClient(c.conn) }

func (c *sessionClient) ServiceAccount() serviceaccountv1.ServiceAccountAPIClient {
	return serviceaccountv1.NewServiceAccountAPIClient(c.conn)
}

func (c *sessionClient) Issuer() serviceaccountv1.IssuerAPIClient {
	return serviceaccountv1.NewIssuerAPIClient(c.conn)
}

func (c *sessionClient) Organization() organizationv1.OrganizationAPIClient {
	return organizationv1.NewOrganizationAPIClient(c.conn)
}

func (c *sessionClient) User() userv1.UserAPIClient { return userv1.NewUserAPIClient(c.conn) }

func (c *sessionClient) ValidateToken() error { return nil } // JWTs are validated by the server

func (c *sessionClient) Version() string { return sdkclient.Version() }

func (c *sessionClient) Close() error { return c.conn.Close() }
