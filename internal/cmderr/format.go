package cmderr

import (
	"errors"
	"strings"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ScopeHint is the remedy line for a PermissionDenied: printed after the
// top-level error, and as the closing hint of a describe view that could
// not load one of its sections.
const ScopeHint = "Run 'admiral auth status' to see the active credential and its scopes."

// Format returns the user-facing message for err, for the top-level
// `Error:` line and for inline notes such as an unavailable describe
// section. For gRPC status errors
// it returns the underlying description verbatim, stripping only the
// "rpc error: code = X desc = " wire framing. Anything beyond that
// (low-signal wrappers, nested framing leaked into descriptions) is a
// server-side message-quality concern and should be addressed there, not
// papered over in the CLI.
func Format(err error) string {
	if err == nil {
		return ""
	}
	s, prefix, ok := innermostStatus(err)
	if !ok {
		return err.Error()
	}

	msg := strings.TrimSpace(s.Message())
	switch s.Code() {
	case codes.DeadlineExceeded:
		return "request timed out; for slow operations raise the limit with ADMIRAL_TIMEOUT (e.g. 5m)"
	case codes.Unavailable:
		return "could not reach the Admiral API: " + msg
	case codes.Unauthenticated:
		if msg == "" {
			return "not signed in"
		}
		return "not signed in: " + msg
	case codes.PermissionDenied:
		if msg == "" {
			return prefix + "permission denied"
		}
		return prefix + "permission denied: " + msg
	}
	if msg == "" {
		return prefix + codeWords(s.Code())
	}
	return prefix + msg
}

// innermostStatus finds the deepest gRPC status in err's chain and the
// context callers wrapped around it ("listing environments: "). Callers
// annotate RPC failures with fmt.Errorf("...: %w", err); status.FromError
// on such a wrapper keeps the code but sets the message to err.Error(),
// which puts "rpc error: code = … desc = …" back in front of the user.
func innermostStatus(err error) (st *status.Status, prefix string, ok bool) {
	var found error
	for e := err; e != nil; e = errors.Unwrap(e) {
		if _, isStatus := e.(interface{ GRPCStatus() *status.Status }); isStatus {
			found = e
		}
	}
	if found == nil {
		return nil, "", false
	}
	st, _ = status.FromError(found)
	return st, strings.TrimSuffix(err.Error(), found.Error()), true
}

// codeWords renders a gRPC code as lowercase words for the rare status that
// arrives with no description (typically a 403/404 from the edge proxy that
// never set grpc-message): NotFound → "not found", AlreadyExists →
// "already exists". Style guide §6.1 forbids Go identifiers in error lines.
func codeWords(c codes.Code) string {
	switch c {
	case codes.Internal:
		return "internal server error"
	case codes.Unknown:
		return "unknown error"
	}
	var b strings.Builder
	for i, r := range c.String() {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
