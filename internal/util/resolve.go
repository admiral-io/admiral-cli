package util

import (
	"context"
	"fmt"

	applicationv1 "go.admiral.io/sdk/proto/admiral/application/v1"
	userv1 "go.admiral.io/sdk/proto/admiral/user/v1"
)

// ResolveAppID resolves an application identifier to its UUID.
// If idFlag is set, it is returned directly. Otherwise, the name is looked up
// via the list endpoint with a name filter.
func ResolveAppID(ctx context.Context, appClient applicationv1.ApplicationAPIClient, name, idFlag string) (string, error) {
	if idFlag != "" {
		return idFlag, nil
	}
	if name == "" {
		return "", fmt.Errorf("no application name or ID provided")
	}

	resp, err := appClient.ListApplications(ctx, &applicationv1.ListApplicationsRequest{
		Filter: fmt.Sprintf("field['name'] = '%s'", name),
	})
	if err != nil {
		return "", fmt.Errorf("looking up application %q: %w", name, err)
	}

	switch len(resp.Applications) {
	case 0:
		return "", fmt.Errorf("application %q not found", name)
	case 1:
		return resp.Applications[0].Id, nil
	default:
		return "", fmt.Errorf("multiple applications match name %q; use --id to specify", name)
	}
}

// ResolvePersonalAccessTokenID resolves a personal access token identifier to its UUID.
// If idFlag is set, it is returned directly. Otherwise, the name is looked up
// via the list endpoint with a name filter (with client-side fallback).
func ResolvePersonalAccessTokenID(ctx context.Context, userClient userv1.UserAPIClient, name, idFlag string) (string, error) {
	if idFlag != "" {
		return idFlag, nil
	}
	if name == "" {
		return "", fmt.Errorf("no token name or ID provided")
	}

	resp, err := userClient.ListPersonalAccessTokens(ctx, &userv1.ListPersonalAccessTokensRequest{
		Filter: fmt.Sprintf("field['name'] = '%s'", name),
	})
	if err != nil {
		return "", fmt.Errorf("looking up token %q: %w", name, err)
	}

	// Client-side filter: the API may not apply the name filter.
	var matched []string
	for _, t := range resp.AccessTokens {
		if t.Name == name {
			matched = append(matched, t.Id)
		}
	}

	switch len(matched) {
	case 0:
		return "", fmt.Errorf("token %q not found", name)
	case 1:
		return matched[0], nil
	default:
		return "", fmt.Errorf("multiple tokens match name %q; use --id to specify", name)
	}
}
