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

// isCobraFlagError recognizes the flag validation errors cobra raises
// itself, which do not pass through the flag error func: "required flag(s)
// ... not set" and the flag-group checks behind MarkFlagsMutuallyExclusive,
// MarkFlagsRequiredTogether and MarkFlagsOneRequired.
func isCobraFlagError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "required flag(s)") ||
		strings.HasPrefix(msg, "if any flags in the group") ||
		strings.HasPrefix(msg, "at least one of the flags in the group")
}

// exitCode maps err to the process exit status: cmderr codes when present,
// 4 for anything that needs a sign-in, 2 for a flag mistake cobra caught,
// otherwise 1.
func exitCode(err error) int {
	if isAuthError(err) {
		return cmderr.ExitAuth
	}
	if isCobraFlagError(err) {
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
