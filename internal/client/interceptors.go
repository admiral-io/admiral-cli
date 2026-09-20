package client

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/credentials"
)

// testDialOptions lets tests point the client at an in-process server.
var testDialOptions []grpc.DialOption

// retryBackoff is the wait before each re-attempt in unavailableRetryInterceptor.
var retryBackoff = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond}

// DefaultTimeout bounds a single RPC when the caller set no deadline. Every
// Admiral RPC is unary, so this applies to all of them; a command that makes
// several calls gets the budget per call, not in total.
const DefaultTimeout = 60 * time.Second

// deadlineInterceptor applies timeout to any call whose context carries no
// deadline of its own. A hung connection or a stalled server then surfaces
// as DeadlineExceeded instead of an indefinite wait.
func deadlineInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, has := ctx.Deadline(); !has && timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// unavailableRetryInterceptor re-attempts read-only calls that fail with
// Unavailable, the code gRPC uses for a dropped connection or a backend
// briefly out of rotation. Only reads are retried: a mutation that reached
// the server before the connection broke would otherwise run twice.
func unavailableRetryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil || !isReadOnly(method) {
			return err
		}
		for _, wait := range retryBackoff {
			if status.Code(err) != codes.Unavailable {
				return err
			}
			slog.Debug("transient failure; retrying", "method", method, "wait", wait, "error", err)
			select {
			case <-ctx.Done():
				return err
			case <-time.After(wait):
			}
			err = invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}
		}
		return err
	}
}

// debugLogInterceptor records each attempt at debug level: method, outcome,
// and duration. It sits innermost so retries show up as separate lines, and
// it is installed on both credential paths so -v behaves the same for an
// API key and a session.
func debugLogInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !slog.Default().Enabled(ctx, slog.LevelDebug) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		slog.Debug("rpc",
			"method", method,
			"code", status.Code(err).String(),
			"duration", time.Since(start).Round(time.Millisecond),
			"target", cc.Target(),
		)
		return err
	}
}

// isReadOnly reports whether a full method name ("/pkg.Service/Method")
// denotes a read, by the API's naming convention.
func isReadOnly(fullMethod string) bool {
	name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
	return strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List")
}

// authRetryInterceptor retries a call once after an Unauthenticated response,
// first force-refreshing the stored session and installing the new token via
// setToken so this and every later call carry it. The token looked valid
// locally but the server rejected it, so a refresh is the only useful
// recovery. One retry per client lifetime keeps a genuinely revoked session
// from looping.
func authRetryInterceptor(configDir string, setToken func(string)) grpc.UnaryClientInterceptor {
	var retried atomic.Bool

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil || status.Code(err) != codes.Unauthenticated {
			return err
		}
		if !retried.CompareAndSwap(false, true) {
			return err
		}

		slog.Debug("unauthenticated response; refreshing session and retrying", "method", method)
		fresh, refreshErr := credentials.ForceRefresh(ctx, configDir)
		if refreshErr != nil {
			slog.Debug("session refresh failed", "error", refreshErr)
			return err
		}
		setToken(fresh.Token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
