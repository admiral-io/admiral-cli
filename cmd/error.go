package cmd

import (
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
)

// formatError returns the user-facing message for err. See cmderr.Format.
func formatError(err error) string { return cmderr.Format(err) }

// isAuthError reports whether err means the user must sign in: a missing or
// expired stored credential, or an RPC the server refused as unauthenticated.
func isAuthError(err error) bool {
	if errors.Is(err, credentials.ErrNotAuthenticated) || errors.Is(err, credentials.ErrSessionExpired) {
		return true
	}
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.Unauthenticated
}

// isRequiredFlagError recognizes cobra's own "required flag(s) ... not set",
// which does not pass through the flag error func.
func isRequiredFlagError(err error) bool {
	return strings.HasPrefix(err.Error(), "required flag(s)")
}

// exitCode maps err to the process exit status: cmderr codes when present,
// 4 for anything that needs a sign-in, 2 for a missing required flag,
// otherwise 1.
func exitCode(err error) int {
	if isAuthError(err) {
		return cmderr.ExitAuth
	}
	if isRequiredFlagError(err) {
		return cmderr.ExitUsage
	}
	return cmderr.Code(err)
}

// errorHint returns the remedy line printed after the error, or "".
func errorHint(err error) string {
	if h := cmderr.Hint(err); h != "" {
		return h
	}
	if isAuthError(err) {
		return "Run 'admiral auth login' to sign in, or set " + credentials.EnvAPIKey + "."
	}
	if status.Code(err) == codes.PermissionDenied {
		return cmderr.ScopeHint
	}
	return ""
}
