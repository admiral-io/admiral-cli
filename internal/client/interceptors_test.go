package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// invokerFunc builds a grpc.UnaryInvoker returning the given errors in
// sequence and recording each call's context.
func invokerFunc(errs ...error) (grpc.UnaryInvoker, *[]context.Context) {
	var seen []context.Context
	return func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		seen = append(seen, ctx)
		i := len(seen) - 1
		if i < len(errs) {
			return errs[i]
		}
		return nil
	}, &seen
}

func TestDeadlineInterceptor_AddsDeadline(t *testing.T) {
	inv, seen := invokerFunc(nil)
	err := deadlineInterceptor(time.Minute)(context.Background(), "/svc/Get", nil, nil, nil, inv)
	require.NoError(t, err)

	dl, ok := (*seen)[0].Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Minute), dl, 2*time.Second)
}

func TestDeadlineInterceptor_KeepsExistingDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	want, _ := ctx.Deadline()

	inv, seen := invokerFunc(nil)
	require.NoError(t, deadlineInterceptor(time.Minute)(ctx, "/svc/Get", nil, nil, nil, inv))

	got, _ := (*seen)[0].Deadline()
	require.Equal(t, want, got, "a caller-supplied deadline is not overridden")
}

func TestIsReadOnly(t *testing.T) {
	require.True(t, isReadOnly("/admiral.api.application.v1.ApplicationAPI/GetApplication"))
	require.True(t, isReadOnly("/admiral.api.run.v1.RunAPI/ListRuns"))
	require.False(t, isReadOnly("/admiral.api.application.v1.ApplicationAPI/CreateApplication"))
	require.False(t, isReadOnly("/admiral.api.run.v1.RunAPI/CancelRun"))
	require.False(t, isReadOnly("/admiral.api.changeset.v1.ChangeSetAPI/ApplyChangeSet"))
}

func fastBackoff(t *testing.T) {
	t.Helper()
	prev := retryBackoff
	retryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryBackoff = prev })
}

func TestUnavailableRetry_RetriesReadsThenSucceeds(t *testing.T) {
	fastBackoff(t)
	unavailable := status.Error(codes.Unavailable, "connection reset")
	inv, seen := invokerFunc(unavailable, unavailable, nil)

	err := unavailableRetryInterceptor()(context.Background(), "/svc/ListThings", nil, nil, nil, inv)
	require.NoError(t, err)
	require.Len(t, *seen, 3)
}

func TestUnavailableRetry_GivesUpAfterBudget(t *testing.T) {
	fastBackoff(t)
	unavailable := status.Error(codes.Unavailable, "still down")
	inv, seen := invokerFunc(unavailable, unavailable, unavailable, unavailable)

	err := unavailableRetryInterceptor()(context.Background(), "/svc/GetThing", nil, nil, nil, inv)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Len(t, *seen, 1+len(retryBackoff))
}

func TestUnavailableRetry_NeverRetriesMutations(t *testing.T) {
	fastBackoff(t)
	inv, seen := invokerFunc(status.Error(codes.Unavailable, "connection reset"), nil)

	err := unavailableRetryInterceptor()(context.Background(), "/svc/CreateThing", nil, nil, nil, inv)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Len(t, *seen, 1, "a create that may have reached the server must not run twice")
}

func TestUnavailableRetry_OnlyForUnavailable(t *testing.T) {
	fastBackoff(t)
	inv, seen := invokerFunc(status.Error(codes.NotFound, "no such thing"), nil)

	err := unavailableRetryInterceptor()(context.Background(), "/svc/GetThing", nil, nil, nil, inv)
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Len(t, *seen, 1)
}

func TestUnavailableRetry_StopsOnCancel(t *testing.T) {
	prev := retryBackoff
	retryBackoff = []time.Duration{time.Hour}
	t.Cleanup(func() { retryBackoff = prev })

	ctx, cancel := context.WithCancel(context.Background())
	unavailable := status.Error(codes.Unavailable, "down")
	inv, seen := invokerFunc(unavailable, nil)

	done := make(chan error, 1)
	go func() { done <- unavailableRetryInterceptor()(ctx, "/svc/GetThing", nil, nil, nil, inv) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.True(t, errors.Is(err, unavailable) || status.Code(err) == codes.Unavailable)
		require.Len(t, *seen, 1, "cancel during backoff must not fire another attempt")
	case <-time.After(2 * time.Second):
		t.Fatal("interceptor did not return after cancel")
	}
}
