package cmd

import (
	"regexp"
	"strings"

	"google.golang.org/grpc/status"
)

type exitError struct {
	err  error
	code int
}

func (e *exitError) Error() string {
	return e.err.Error()
}

// rpcErrRe matches "rpc error: code = <Code> desc = <message>" fragments.
var rpcErrRe = regexp.MustCompile(`rpc error: code = \w+ desc = `)

// formatError returns a user-friendly error message. For gRPC status errors
// it extracts the deepest description, stripping all "rpc error: ..." framing.
func formatError(err error) string {
	msg := err.Error()

	// If it doesn't look like a gRPC error, return as-is.
	s, ok := status.FromError(err)
	if !ok {
		return msg
	}

	// Strip all "rpc error: code = Xxx desc = " prefixes, keeping only the
	// final human-readable message.
	cleaned := rpcErrRe.ReplaceAllString(msg, "")

	// When errors are chained (e.g. "failed to create application: <rpc>"),
	// we may end up with "failed to create application: Application ...".
	// Find the last colon-separated segment that isn't just whitespace.
	parts := strings.Split(cleaned, ": ")
	if len(parts) > 1 {
		cleaned = strings.TrimSpace(parts[len(parts)-1])
	} else {
		cleaned = strings.TrimSpace(cleaned)
	}

	// If the description was empty, fall back to the gRPC status code.
	if cleaned == "" {
		return s.Code().String()
	}

	return cleaned
}
