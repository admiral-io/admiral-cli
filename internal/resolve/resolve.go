// Package resolve turns the name-or-ID a user typed into the UUID the API
// wants. A UUID is used as-is; anything else is looked up by name inside
// its parent scope. Misses and ambiguities are typed errors that carry the
// scope and the command that lists the candidates, so the root can print a
// remedy line.
package resolve

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	commonv1 "buf.build/gen/go/admiral/common/protocolbuffers/go/admiral/common/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/filter"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	credentialv1 "go.admiral.io/sdk/proto/admiral/api/credential/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/api/source/v1"
	userv1 "go.admiral.io/sdk/proto/admiral/api/user/v1"
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID and therefore a server ID
// rather than a name.
func IsUUID(s string) bool { return uuidRE.MatchString(s) }

// NotFoundError is returned when a name matches nothing in its scope.
type NotFoundError struct {
	Kind  string // "environment"
	Name  string // what the user typed
	Scope string // `in application "shop"`, or ""
	// ListCommand is the admiral command that lists the candidates, used
	// for the hint: "env list --app shop".
	ListCommand string
}

func (e *NotFoundError) Error() string {
	if e.Scope != "" {
		return fmt.Sprintf("%s %q not found %s", e.Kind, e.Name, e.Scope)
	}
	return fmt.Sprintf("%s %q not found", e.Kind, e.Name)
}

// Hint names the command that shows what does exist.
func (e *NotFoundError) Hint() string {
	if e.ListCommand == "" {
		return ""
	}
	return fmt.Sprintf("Run 'admiral %s' to see %ss.", e.ListCommand, e.Kind)
}

// AmbiguousError is returned when a name matches more than one resource.
type AmbiguousError struct {
	Kind string
	Name string
	IDs  []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%d %ss are named %q", len(e.IDs), e.Kind, e.Name)
}

// Hint lists the candidate IDs so the user can pass one instead.
func (e *AmbiguousError) Hint() string {
	return "Pass an ID instead: " + strings.Join(e.IDs, ", ")
}

// App resolves an application name or ID.
func App(ctx context.Context, c applicationv1.ApplicationAPIClient, nameOrID string) (string, error) {
	if nameOrID == "" {
		return "", cmderr.UsageHint("Pass --app.", "no application specified")
	}
	return byName(ctx, "application", nameOrID, "", "app list", "",
		func(ctx context.Context, f string) ([]*applicationv1.Application, error) {
			resp, err := c.ListApplications(ctx, &applicationv1.ListApplicationsRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.Applications, nil
		},
		func(a *applicationv1.Application) string { return a.Name },
		func(a *applicationv1.Application) string { return a.Id },
	)
}

// Environment resolves an environment name or ID. A UUID needs no
// application; a name is looked up inside app (itself a name or ID). The
// caller has already split an app/env path with flags.EnvTarget.
func Environment(ctx context.Context, envs environmentv1.EnvironmentAPIClient, apps applicationv1.ApplicationAPIClient, app, nameOrID string) (string, error) {
	if IsUUID(nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.UsageHint("Pass --env as a name or app/env.", "no environment specified")
	}
	if app == "" {
		return "", cmderr.UsageHint("Pass --app or give the environment as app/env.", "no application specified")
	}
	appID, err := App(ctx, apps, app)
	if err != nil {
		return "", err
	}
	scopeFilter, err := filter.Eq("application_id", appID)
	if err != nil {
		return "", err
	}
	return byName(ctx, "environment", nameOrID,
		fmt.Sprintf("in application %q", app), "env list --app "+app, scopeFilter,
		func(ctx context.Context, f string) ([]*environmentv1.Environment, error) {
			resp, err := envs.ListEnvironments(ctx, &environmentv1.ListEnvironmentsRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.Environments, nil
		},
		func(e *environmentv1.Environment) string { return e.Name },
		func(e *environmentv1.Environment) string { return e.Id },
	)
}

// Credential resolves a credential name or ID.
func Credential(ctx context.Context, c credentialv1.CredentialAPIClient, nameOrID string) (string, error) {
	return byName(ctx, "credential", nameOrID, "", "credential list", "",
		func(ctx context.Context, f string) ([]*credentialv1.Credential, error) {
			resp, err := c.ListCredentials(ctx, &credentialv1.ListCredentialsRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.Credentials, nil
		},
		func(c *credentialv1.Credential) string { return c.Name },
		func(c *credentialv1.Credential) string { return c.Id },
	)
}

// Source resolves a source name or ID.
func Source(ctx context.Context, c sourcev1.SourceAPIClient, nameOrID string) (string, error) {
	return byName(ctx, "source", nameOrID, "", "source list", "",
		func(ctx context.Context, f string) ([]*sourcev1.Source, error) {
			resp, err := c.ListSources(ctx, &sourcev1.ListSourcesRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.Sources, nil
		},
		func(s *sourcev1.Source) string { return s.Name },
		func(s *sourcev1.Source) string { return s.Id },
	)
}

// Agent resolves an agent name or ID.
func Agent(ctx context.Context, c agentv1.AgentAPIClient, nameOrID string) (string, error) {
	return byName(ctx, "agent", nameOrID, "", "agent list", "",
		func(ctx context.Context, f string) ([]*agentv1.Agent, error) {
			resp, err := c.ListAgents(ctx, &agentv1.ListAgentsRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.Agents, nil
		},
		func(a *agentv1.Agent) string { return a.Name },
		func(a *agentv1.Agent) string { return a.Id },
	)
}

// AgentToken resolves an agent token name or ID. A UUID needs no agent; a
// name is looked up inside agent (itself a name or ID).
func AgentToken(ctx context.Context, c agentv1.AgentAPIClient, agent, nameOrID string) (string, error) {
	if IsUUID(nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.Usage("no token specified")
	}
	if agent == "" {
		return "", cmderr.UsageHint("Pass the agent name, or the token's ID instead of its name.",
			"an agent is required to look up token %q by name", nameOrID)
	}
	agentID, err := Agent(ctx, c, agent)
	if err != nil {
		return "", err
	}
	return byName(ctx, "token", nameOrID,
		fmt.Sprintf("on agent %q", agent), "agent token list --agent "+agent, "",
		func(ctx context.Context, f string) ([]*commonv1.ApiKey, error) {
			resp, err := c.ListApiKeys(ctx, &agentv1.ListApiKeysRequest{AgentId: agentID, Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.ApiKeys, nil
		},
		func(t *commonv1.ApiKey) string { return t.Name },
		func(t *commonv1.ApiKey) string { return t.Id },
	)
}

// PersonalAccessToken resolves a personal API key name or ID.
func PersonalAccessToken(ctx context.Context, c userv1.UserAPIClient, nameOrID string) (string, error) {
	return byName(ctx, "API key", nameOrID, "", "auth key list", "",
		func(ctx context.Context, f string) ([]*commonv1.ApiKey, error) {
			resp, err := c.ListApiKeys(ctx, &userv1.ListApiKeysRequest{Filter: f})
			if err != nil {
				return nil, err
			}
			return resp.ApiKeys, nil
		},
		func(t *commonv1.ApiKey) string { return t.Name },
		func(t *commonv1.ApiKey) string { return t.Id },
	)
}

// byName looks nameOrID up in a collection. list is called with a server
// filter that combines scope with the name; results are re-checked
// client-side so servers that ignore the filter still produce correct
// answers.
func byName[T any](
	ctx context.Context,
	kind, nameOrID, scope, listCommand, scopeFilter string,
	list func(ctx context.Context, filter string) ([]T, error),
	nameOf func(T) string,
	idOf func(T) string,
) (string, error) {
	if IsUUID(nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.Usage("no %s specified", kind)
	}

	byName, err := filter.Eq("name", nameOrID)
	if err != nil {
		return "", err
	}
	items, err := list(ctx, filter.And(scopeFilter, byName))
	if err != nil {
		return "", fmt.Errorf("looking up %s %q: %w", kind, nameOrID, err)
	}

	var ids []string
	for _, it := range items {
		if nameOf(it) == nameOrID {
			ids = append(ids, idOf(it))
		}
	}
	switch len(ids) {
	case 0:
		return "", &NotFoundError{Kind: kind, Name: nameOrID, Scope: scope, ListCommand: listCommand}
	case 1:
		return ids[0], nil
	default:
		return "", &AmbiguousError{Kind: kind, Name: nameOrID, IDs: ids}
	}
}

// Component resolves a registry component name or ID. Names are unique in
// the registry, so this is one read rather than a filtered list.
func Component(ctx context.Context, c registryv1.RegistryAPIClient, nameOrID string) (string, error) {
	if IsUUID(nameOrID) {
		return nameOrID, nil
	}
	if nameOrID == "" {
		return "", cmderr.Usage("no component specified")
	}
	resp, err := c.GetComponent(ctx, &registryv1.GetComponentRequest{Name: nameOrID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", &NotFoundError{Kind: "component", Name: nameOrID, ListCommand: "component list"}
		}
		return "", fmt.Errorf("looking up component %q: %w", nameOrID, err)
	}
	return resp.Component.Id, nil
}
