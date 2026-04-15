package util

import (
	"context"
	"fmt"

	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
	credentialv1 "go.admiral.io/sdk/proto/admiral/credential/v1"
	sourcev1 "go.admiral.io/sdk/proto/admiral/source/v1"
	commonv1 "go.admiral.io/sdk/proto/admiral/common/v1"
	userv1 "go.admiral.io/sdk/proto/admiral/user/v1"
)

// Resolve returns a resource UUID given either an explicit idFlag or a name.
// If idFlag is non-empty it is returned verbatim (no RPC). Otherwise list is
// invoked with a name filter and the results are re-filtered client-side via
// nameOf so servers that ignore the filter still produce correct results.
// resource labels errors (e.g. "application", "credential").
func Resolve[T any](
	ctx context.Context,
	resource, name, idFlag string,
	list func(ctx context.Context, filter string) ([]T, error),
	nameOf func(T) string,
	idOf func(T) string,
) (string, error) {
	if idFlag != "" {
		return idFlag, nil
	}
	if name == "" {
		return "", fmt.Errorf("no %s name or ID provided", resource)
	}

	items, err := list(ctx, fmt.Sprintf("field['name'] = '%s'", name))
	if err != nil {
		return "", fmt.Errorf("looking up %s %q: %w", resource, name, err)
	}

	var matched []string
	for _, it := range items {
		if nameOf(it) == name {
			matched = append(matched, idOf(it))
		}
	}
	switch len(matched) {
	case 0:
		return "", fmt.Errorf("%s %q not found", resource, name)
	case 1:
		return matched[0], nil
	default:
		return "", fmt.Errorf("multiple %ss match name %q; use --id to specify", resource, name)
	}
}

// ResolveAppID resolves an application name or UUID to its UUID.
func ResolveAppID(ctx context.Context, c applicationv1.ApplicationAPIClient, name, idFlag string) (string, error) {
	return Resolve(ctx, "application", name, idFlag,
		func(ctx context.Context, filter string) ([]*applicationv1.Application, error) {
			resp, err := c.ListApplications(ctx, &applicationv1.ListApplicationsRequest{Filter: filter})
			if err != nil {
				return nil, err
			}
			return resp.Applications, nil
		},
		func(a *applicationv1.Application) string { return a.Name },
		func(a *applicationv1.Application) string { return a.Id },
	)
}

// ResolveCredentialID resolves a credential name or UUID to its UUID.
func ResolveCredentialID(ctx context.Context, c credentialv1.CredentialAPIClient, name, idFlag string) (string, error) {
	return Resolve(ctx, "credential", name, idFlag,
		func(ctx context.Context, filter string) ([]*credentialv1.Credential, error) {
			resp, err := c.ListCredentials(ctx, &credentialv1.ListCredentialsRequest{Filter: filter})
			if err != nil {
				return nil, err
			}
			return resp.Credentials, nil
		},
		func(c *credentialv1.Credential) string { return c.Name },
		func(c *credentialv1.Credential) string { return c.Id },
	)
}

// ResolveSourceID resolves a source name or UUID to its UUID.
func ResolveSourceID(ctx context.Context, c sourcev1.SourceAPIClient, name, idFlag string) (string, error) {
	return Resolve(ctx, "source", name, idFlag,
		func(ctx context.Context, filter string) ([]*sourcev1.Source, error) {
			resp, err := c.ListSources(ctx, &sourcev1.ListSourcesRequest{Filter: filter})
			if err != nil {
				return nil, err
			}
			return resp.Sources, nil
		},
		func(s *sourcev1.Source) string { return s.Name },
		func(s *sourcev1.Source) string { return s.Id },
	)
}

// ResolvePersonalAccessTokenID resolves a PAT name or UUID to its UUID.
func ResolvePersonalAccessTokenID(ctx context.Context, c userv1.UserAPIClient, name, idFlag string) (string, error) {
	return Resolve(ctx, "token", name, idFlag,
		func(ctx context.Context, filter string) ([]*commonv1.AccessToken, error) {
			resp, err := c.ListPersonalAccessTokens(ctx, &userv1.ListPersonalAccessTokensRequest{Filter: filter})
			if err != nil {
				return nil, err
			}
			return resp.AccessTokens, nil
		},
		func(t *commonv1.AccessToken) string { return t.Name },
		func(t *commonv1.AccessToken) string { return t.Id },
	)
}