package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFormatError_NonGRPC(t *testing.T) {
	err := errors.New("plain error from somewhere")
	require.Equal(t, "plain error from somewhere", formatError(err))
}

func TestFormatError_NilNotFromStatus(t *testing.T) {
	// status.FromError on a nil error returns OK; formatError isn't expected
	// to be called with nil but should not panic if it is.
	require.NotPanics(t, func() { _ = formatError(nil) })
}

func TestFormatError_StripsFraming(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "invalid change_set_id: invalid UUID length: 15")
	require.Equal(t, "invalid change_set_id: invalid UUID length: 15", formatError(err))
}

func TestFormatError_PreservesChainedDescription(t *testing.T) {
	// Runbook §8a regression: the CLI was destroying the message by greedy
	// split. The component name and template path must survive verbatim.
	desc := `failed to evaluate values_template for component "database": template evaluation error: template: values:2:28: executing "values" at <.component.network.vpc_id>: map has no entry for key "component"`
	err := status.Error(codes.InvalidArgument, desc)
	require.Equal(t, desc, formatError(err))
}

func TestFormatError_EmptyDescFallsBackToCodeWords(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want string
	}{
		{codes.NotFound, "not found"},
		{codes.AlreadyExists, "already exists"},
		{codes.FailedPrecondition, "failed precondition"},
		{codes.Internal, "internal server error"},
		{codes.Unknown, "unknown error"},
	} {
		require.Equal(t, tc.want, formatError(status.Error(tc.code, "")), tc.code.String())
	}
}

func TestFormatError_PermissionDenied(t *testing.T) {
	require.Equal(t, "permission denied", formatError(status.Error(codes.PermissionDenied, "")))
	require.Equal(t, `permission denied: missing required scope "app:read"`,
		formatError(status.Error(codes.PermissionDenied, `missing required scope "app:read"`)))
	require.Equal(t, "Run 'admiral auth status' to see the active credential and its scopes.",
		errorHint(status.Error(codes.PermissionDenied, "")))
	require.Equal(t, 1, exitCode(status.Error(codes.PermissionDenied, "")))
}

func TestFormatError_TrimsSurroundingWhitespace(t *testing.T) {
	err := status.Error(codes.Internal, "  something bad  \n")
	require.Equal(t, "something bad", formatError(err))
}

func TestFormatError_WrappedStatusKeepsContextDropsFraming(t *testing.T) {
	inner := status.Error(codes.Unimplemented, "not implemented")
	err := fmt.Errorf("listing environments: %w", inner)
	require.Equal(t, "listing environments: not implemented", formatError(err))

	// Two levels of wrapping, empty description.
	err = fmt.Errorf("describe: %w", fmt.Errorf("listing runs: %w", status.Error(codes.NotFound, "")))
	require.Equal(t, "describe: listing runs: not found", formatError(err))

	// Exit code and hint still see the underlying code through the wrapper.
	require.Equal(t, 4, exitCode(fmt.Errorf("ctx: %w", status.Error(codes.Unauthenticated, ""))))
}
